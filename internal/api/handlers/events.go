package handlers

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"litcontainer/internal/daemon"
	"litcontainer/internal/events"
	"litcontainer/internal/logger"
	"net/http"
)

type EventsHandler struct {
	daemon *daemon.Daemon
}

func NewEventsHandler(d *daemon.Daemon) *EventsHandler {
	return &EventsHandler{daemon: d}
}

func (h *EventsHandler) StreamEvents(c *gin.Context) {
	c.Header("Content-Type", "application/json")
	c.Status(http.StatusOK)

	ch, unsub := h.daemon.GetEventBus().Subscribe(events.DefaultBufferSize)
	defer unsub()

	fw := &flushingWriter{w: c.Writer}
	enc := json.NewEncoder(fw)
	ctx := c.Request.Context()

	for {
		select {
		case event, ok := <-ch:
			if !ok {
				logger.Warn("events channel closed")
				return
			}
			// 事件通过json编码后写入响应流，客户端可以实时接收事件
			if err := enc.Encode(event); err != nil {
				logger.Warn("encode event: %v", err)
				return
			}
		case <-ctx.Done():
			logger.Info("events stream client disconnected")
			return
		}
	}
}
