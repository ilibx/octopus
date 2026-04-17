package matrix

import (
	"github.com/ilibx/octopus/pkg/bus"
	"github.com/ilibx/octopus/pkg/channels"
	"github.com/ilibx/octopus/pkg/config"
)

func init() {
	channels.RegisterFactory("matrix", func(cfg *config.Config, b *bus.MessageBus) (channels.Channel, error) {
		return NewMatrixChannel(cfg.Channels.Matrix, b)
	})
}
