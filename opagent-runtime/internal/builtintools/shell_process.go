package builtintools

type shellInvocation struct {
	Executable  string
	Args        []string
	DisplayName string
}

func resolveShellInvocation(command string) shellInvocation {
	return defaultShellInvocation(command)
}
