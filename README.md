# Build and run

Run `make` to build the system

Executable will be built to `build/alphabill`.

Run the executable `alphabill shard` to start shard node with default configuration. To see possible configuration options run with `--help` flag.

## Configuration

It's possible to define the configuration values from (in the order of precedence):
* Command line flags (e.g. `--initial-bill-value=1000`)
* Environment (Prefix 'AB' must be used. E.g. `AB_INITIAL_BILL_VALUE=1000`)
* Configuration file (properties file) (E.g. `initial-bill-value=1000`)
* Default values

# CI setup

See gitlab-ci.yml for details.

GitLab runs the CI job inside docker container defined in `deployments/gitlab`.
