package onebot

import (
	"github.com/ilibx/octopus/pkg/bus"
	"github.com/ilibx/octopus/pkg/channels"
	"github.com/ilibx/octopus/pkg/config"
)

func init() {
	channels.RegisterFactory("onebot", func(cfg *config.Config, b *bus.MessageBus) (channels.Channel, error) {
		return NewOneBotChannel(cfg.Channels.OneBot, b)
	})
}
