package shim

import "encoding/json"

const (
	CmdState = "state"
	CmdStop  = "stop"
	CmdWait  = "wait"
)

type Request struct {
	Cmd  string          `json:"cmd"`
	Args json.RawMessage `json:"args,omitempty"`
}
type Response struct {
	OK    bool            `json:"ok"`
	Error string          `json:"error,omitempty"`
	Data  json.RawMessage `json:"data,omitempty"`
}
type StopArgs struct {
	Signal  int `json:"signal"`  // 默认 SIGTERM
	Timeout int `json:"timeout"` // 秒，超时后 SIGKILL
}
type StateData struct {
	ID     string `json:"id"`
	Pid    int    `json:"pid"`
	Status string `json:"status"`
	Exit   int    `json:"exit,omitempty"`
}
