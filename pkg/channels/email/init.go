package email

import (
	"github.com/ilibx/octopus/pkg/bus"
	"github.com/ilibx/octopus/pkg/channels"
	"github.com/ilibx/octopus/pkg/config"
)

func init() {
	channels.RegisterFactory("email", func(cfg *config.Config, b *bus.MessageBus) (channels.Channel, error) {
		if !cfg.Channels.Email.Enabled {
			return nil, nil
		}
		return NewEmailChannel(cfg.Channels.Email, b)
	})
}
