package client

import (
	"encoding/json"
	"fmt"
	"io"
	"litcontainer/internal/events"
	"net/http"
)

func (c *Client) EventsStream(handler func(event events.Event) error) error {
	// x代替站位,因为是Socket，没有ip
	req, _ := http.NewRequest("GET", "http://x/api/v1/events", nil)
	rsp, err := c.httpClient.Do(req)
	if err != nil {
		return ErrDaemonUnreachable
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(rsp.Body)
		return fmt.Errorf("events request failed: %s", string(body))
	}

	dec := json.NewDecoder(rsp.Body)
	for {
		var e events.Event
		if err := dec.Decode(&e); err != nil {
			if err == io.EOF {
				return nil // 服务器关闭了连接，正常结束
			}
			return fmt.Errorf("decode event: %w", err)
		}
		// 回调处理
		if err := handler(e); err != nil {
			return fmt.Errorf("handle event: %w", err)
		}
	}
}
