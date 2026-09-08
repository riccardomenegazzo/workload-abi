# Security policy

Workload ABI executes container images supplied by the user. Treat compared images as untrusted code and run the tool only against a Docker environment you are willing to use as a sandbox.

Do not pass production credentials through scenarios. Use synthetic fixtures for demonstrations and CI.

For security vulnerabilities in Workload ABI itself, please use GitHub's private vulnerability reporting if enabled for this repository rather than opening a public issue with exploit details.
