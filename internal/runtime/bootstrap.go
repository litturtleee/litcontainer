package runtime

type InitBootstrap struct {
	Spec     *Spec  `json:"spec"`
	FifoPath string `json:"fifoPath"`
}
