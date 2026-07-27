package claygent

func Run(args []string) {
	if len(args) > 0 && (args[0] == "api" || args[0] == "serve") {
		serve(args[1:])
		return
	}
	runCLI(args)
}
