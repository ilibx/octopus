package feishu

import (
	"github.com/ilibx/octopus/pkg/bus"
	"github.com/ilibx/octopus/pkg/channels"
	"github.com/ilibx/octopus/pkg/config"
)

func init() {
	channels.RegisterFactory("feishu", func(cfg *config.Config, b *bus.MessageBus) (channels.Channel, error) {
		return NewFeishuChannel(cfg.Channels.Feishu, b)
	})
}
