package daemon

func leaseIdentity(source, destination, variable string) string {
	return source + ";" + destination + ";" + variable
}
