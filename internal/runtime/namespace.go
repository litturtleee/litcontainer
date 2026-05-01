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
		// path不为空则以setns的方式进入ns
		if ns.Path != "" {
			continue
		}
		flags |= mapping[ns.Type]
	}

	return flags
}
