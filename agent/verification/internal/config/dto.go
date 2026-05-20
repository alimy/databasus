package config

type parsedFlags struct {
	databasusHost     *string
	agentID           *string
	token             *string
	maxCPU            *int
	maxRAMMb          *int
	maxDiskGb         *int
	maxConcurrentJobs *int
	allowInsecureHTTP *bool

	sources map[string]string
}
