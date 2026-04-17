package slack

import (
	"github.com/ilibx/octopus/pkg/bus"
	"github.com/ilibx/octopus/pkg/channels"
	"github.com/ilibx/octopus/pkg/config"
)

func init() {
	channels.RegisterFactory("slack", func(cfg *config.Config, b *bus.MessageBus) (channels.Channel, error) {
		return NewSlackChannel(cfg.Channels.Slack, b)
	})
}
