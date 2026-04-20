package email

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"

	"github.com/ilibx/octopus/pkg/bus"
	"github.com/ilibx/octopus/pkg/channels"
	"github.com/ilibx/octopus/pkg/config"
	"github.com/ilibx/octopus/pkg/identity"
	"github.com/ilibx/octopus/pkg/logger"
)

// EmailChannel implements the Channel interface for IMAP email polling.
type EmailChannel struct {
	*channels.BaseChannel
	config config.EmailConfig

	mu       sync.Mutex
	client   *imapclient.Client
	ctx      context.Context
	cancel   context.CancelFunc
	pollDone chan struct{}

	lastUID imap.UID // Track last seen UID to avoid re-processing
}

// NewEmailChannel creates a new email channel.
func NewEmailChannel(cfg config.EmailConfig, messageBus *bus.MessageBus) (*EmailChannel, error) {
	if cfg.Server == "" {
		return nil, fmt.Errorf("email server is required")
	}
	if cfg.Username == "" {
		return nil, fmt.Errorf("email username is required")
	}
	if cfg.Password == "" {
		return nil, fmt.Errorf("email password is required")
	}

	base := channels.NewBaseChannel(
		"email",
		cfg,
		messageBus,
		cfg.AllowFrom,
		channels.WithReasoningChannelID(cfg.ReasoningChannelID),
	)

	return &EmailChannel{
		BaseChannel: base,
		config:      cfg,
		pollDone:    make(chan struct{}),
	}, nil
}

// Start begins polling the email server at configured intervals.
func (c *EmailChannel) Start(ctx context.Context) error {
	logger.InfoC("email", "Starting Email channel")
	c.ctx, c.cancel = context.WithCancel(ctx)

	if err := c.connect(); err != nil {
		return fmt.Errorf("email connect failed: %w", err)
	}

	go c.pollLoop()

	c.SetRunning(true)
	logger.InfoCF("email", "Email channel started", map[string]any{
		"server":       c.config.Server,
		"username":     c.config.Username,
		"poll_interval": c.config.PollInterval,
	})
	return nil
}

// Stop disconnects from the email server and stops polling.
func (c *EmailChannel) Stop(ctx context.Context) error {
	logger.InfoC("email", "Stopping Email channel")
	c.SetRunning(false)

	if c.cancel != nil {
		c.cancel()
	}

	// Wait for poll loop to exit
	select {
	case <-c.pollDone:
	case <-time.After(5 * time.Second):
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.client != nil {
		_ = c.client.Close()
		_ = c.client.Logout(context.Background())
	}

	logger.InfoC("email", "Email channel stopped")
	return nil
}

// Send is not applicable for email channel (receive-only).
func (c *EmailChannel) Send(ctx context.Context, msg bus.OutboundMessage) error {
	// Email channel is receive-only for task ingestion
	logger.DebugC("email", "Send called on email channel (receive-only)")
	return nil
}

// connect establishes connection to the IMAP server.
func (c *EmailChannel) connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	server := c.config.Server
	if !strings.Contains(server, ":") {
		if c.config.TLS {
			server += ":993"
		} else {
			server += ":143"
		}
	}

	var dialer imapclient.Dialer
	if c.config.TLS {
		dialer = func(address string) (imapclient.Conn, error) {
			conn, err := tls.Dial("tcp", address, &tls.Config{
				ServerName: strings.Split(c.config.Server, ":")[0],
			})
			if err != nil {
				return nil, err
			}
			return conn, nil
		}
	}

	options := &imapclient.Options{}
	if dialer != nil {
		options.Dialer = dialer
	}

	client, err := imapclient.DialTLS(c.config.Server, options)
	if err != nil {
		return fmt.Errorf("dial failed: %w", err)
	}

	if err := client.Login(c.config.Username, c.config.Password); err != nil {
		_ = client.Close()
		return fmt.Errorf("login failed: %w", err)
	}

	c.client = client
	return nil
}

// pollLoop periodically checks for new emails.
func (c *EmailChannel) pollLoop() {
	defer close(c.pollDone)

	interval := time.Duration(c.config.PollInterval) * time.Second
	if interval <= 0 {
		interval = 60 * time.Second // default 1 minute
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Initial poll
	c.pollOnce()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.pollOnce()
		}
	}
}

// pollOnce performs a single poll cycle to check for new emails.
func (c *EmailChannel) pollOnce() {
	c.mu.Lock()
	client := c.client
	c.mu.Unlock()

	if client == nil {
		logger.WarnC("email", "No email client available, attempting reconnect")
		if err := c.connect(); err != nil {
			logger.ErrorCF("email", "Reconnect failed", map[string]any{"error": err.Error()})
			return
		}
		c.mu.Lock()
		client = c.client
		c.mu.Unlock()
	}

	if client == nil {
		return
	}

	// Select inbox mailbox
	mbox, err := client.Select("INBOX", false)
	if err != nil {
		logger.ErrorCF("email", "Failed to select INBOX", map[string]any{"error": err.Error()})
		// Try to reconnect
		if err := c.connect(); err != nil {
			logger.ErrorCF("email", "Reconnect after select failure failed", map[string]any{"error": err.Error()})
		}
		return
	}

	if mbox.NumMessages == 0 {
		logger.DebugC("email", "No messages in inbox")
		return
	}

	// Fetch all messages (or use UID search for UNSEEN)
	// For simplicity, we fetch recent messages and track by UID
	seqSet := imap.SeqSetNum(1, mbox.NumMessages)
	if c.lastUID > 0 {
		// Only fetch messages with UID > lastUID
		// This requires UID-based fetching
		seqSet = imap.SeqSetNum(mbox.NumMessages) // Fetch latest
	}

	options := &imap.FetchOptions{
		Envelope: true,
		Flags:    true,
		UID:      true,
	}

	messages := client.Fetch(seqSet, options)
	for messages.Next() {
		msg := messages.Message()
		if msg == nil {
			continue
		}

		// Skip already processed messages
		if c.lastUID > 0 && msg.UID <= c.lastUID {
			continue
		}

		// Skip flagged/deleted messages
		if msg.HasFlag(imap.FlagDeleted) || msg.HasFlag(imap.FlagSeen) {
			continue
		}

		// Fetch full message content
		c.processMessage(client, msg.UID)
	}

	if err := messages.Close(); err != nil {
		logger.DebugCF("email", "Error closing messages iterator", map[string]any{"error": err.Error()})
	}
}

// processMessage fetches and processes a single email message.
func (c *EmailChannel) processMessage(client *imapclient.Client, uid imap.UID) {
	seqSet := imap.SeqSetNum(uid)
	options := &imap.FetchOptions{
		Envelope:    true,
		Flags:       true,
		UID:         true,
		BodySection: []*imap.BodySection{{}}, // Fetch entire body
	}

	messages := client.Fetch(seqSet, options)
	if !messages.Next() {
		return
	}
	msg := messages.Message()
	if msg == nil {
		return
	}
	if err := messages.Close(); err != nil {
		logger.DebugCF("email", "Error closing messages iterator", map[string]any{"error": err.Error()})
	}

	if msg.Envelope == nil {
		return
	}

	// Extract subject and body
	subject := msg.Envelope.Subject
	from := ""
	if len(msg.Envelope.From) > 0 {
		from = msg.Envelope.From[0].Address()
	}

	// Decode body
	bodyText := c.extractBody(msg)

	if strings.TrimSpace(bodyText) == "" {
		// No body content, skip
		return
	}

	// Check allow list
	sender := bus.SenderInfo{
		Platform:    "email",
		PlatformID:  from,
		CanonicalID: identity.BuildCanonicalID("email", from),
		Username:    from,
		DisplayName: c.decodeHeader(subject),
	}

	if !c.IsAllowedSender(sender) {
		logger.DebugCF("email", "Sender not in allow list", map[string]any{"from": from})
		return
	}

	chatID := from
	messageID := fmt.Sprintf("email-%d", uid)

	metadata := map[string]string{
		"platform": "email",
		"server":   c.config.Server,
		"subject":  c.decodeHeader(subject),
		"from":     from,
	}

	peer := bus.Peer{Kind: "direct", ID: from}

	c.HandleMessage(c.ctx, peer, messageID, from, chatID, bodyText, nil, metadata, sender)

	// Update last seen UID
	c.lastUID = uid

	// Mark as seen (optional, based on config)
	if c.config.MarkAsRead {
		_ = client.Store(seqSet, imap.FormatFlagsOp(imap.FlagsAdd, true), []imap.Flag{imap.FlagSeen}, nil)
	}
}

// extractBody extracts plain text body from the email.
func (c *EmailChannel) extractBody(msg *imap.FetchResponseData) string {
	if msg.BodySection == nil || len(msg.BodySection) == 0 {
		return ""
	}

	body := msg.BodySection[0].Content
	if body == nil {
		return ""
	}

	data, err := io.ReadAll(body)
	if err != nil {
		logger.DebugCF("email", "Failed to read body", map[string]any{"error": err.Error()})
		return ""
	}

	// Try to decode multipart or quoted-printable
	contentType := msg.BodySection[0].ContentType
	if contentType != "" {
		return c.decodeBody(string(data), contentType)
	}

	return string(data)
}

// decodeBody decodes email body based on content type.
func (c *EmailChannel) decodeBody(data, contentType string) string {
	// Parse content type
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return data
	}

	// Handle multipart
	if strings.HasPrefix(mediaType, "multipart/") {
		return c.extractMultipart(data, params["boundary"])
	}

	// Handle quoted-printable
	if params["charset"] != "" || params["encoding"] == "quoted-printable" {
		reader := quotedprintable.NewReader(strings.NewReader(data))
		decoded, err := io.ReadAll(reader)
		if err == nil {
			return string(decoded)
		}
	}

	return data
}

// extractMultipart extracts plain text part from multipart message.
func (c *EmailChannel) extractMultipart(data, boundary string) string {
	if boundary == "" {
		return data
	}

	reader := multipart.NewReader(strings.NewReader(data), boundary)
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return data
		}

		contentType := part.Header.Get("Content-Type")
		if strings.HasPrefix(contentType, "text/plain") {
			body, err := io.ReadAll(part)
			if err == nil {
				return string(body)
			}
		}
	}

	return data
}

// decodeHeader decodes MIME encoded header (e.g., =?UTF-8?B?...?=).
func (c *EmailChannel) decodeHeader(s string) string {
	decoder := new(mime.WordDecoder)
	decoded, err := decoder.DecodeHeader(s)
	if err != nil {
		return s
	}
	return decoded
}
