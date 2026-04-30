package runtime

import "syscall"

func CloneFlags(namespace []*Namespace) uintptr {
	mapping := map[string]uintptr{
		"pid":     syscall.CLONE_NEWPID,
		"mount":   syscall.CLONE_NEWNS,
		"uts":     syscall.CLONE_NEWUTS,
		"ipc":     syscall.CLONE_NEWIPC,
		"network": syscall.CLONE_NEWNET,
	}
	var flags uintptr

	for _, ns := range namespace {
		flags |= mapping[ns.Type]
	}

	return flags
}
