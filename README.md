# Build

Run `make build` to build the application. Executable will be built to `build/alphabill`. 

### Build dependencies

* `golang` version 1.20. (https://go.dev/doc/install)
* in order to rebuild everything including protobuf definitions (`make` or `make all`):
  * `protoc` version 3.21.9+ (https://grpc.io/docs/protoc-installation)
  * `protoc-gen-go` (https://grpc.io/docs/languages/go/quickstart/)

# Money Partition

1. Run script `start-money.sh` to rebuild the project, start a root chain and 3 money partition nodes.

   The script generates rootchain and partition node keys, genesis files, and starts nodes.
   Node configuration files are located in `testab` directory.

2. Run script `stop-all.sh` to stop the root chain and partition nodes.
   
   Alternatively, use `stop-money.sh` to stop and `resume-money.sh` to resume partition nodes and `stop-root.sh` and `resume-root.sh` to stop and resume the root chain. 

## Configuration

It's possible to define the configuration values from (in the order of precedence):

* Command line flags (e.g. `--address="/ip4/127.0.0.1/tcp/26652"`)
* Environment (Prefix 'AB' must be used. E.g. `AB_ADDRESS="/ip4/127.0.0.1/tcp/26652"`)
* Configuration file (properties file) (E.g. `address="/ip4/127.0.0.1/tcp/26652"`)
* Default values

The default location of configuration file is `$AB_HOME/config.props`

The default `$AB_HOME` is `$HOME/.alphabill`

# Verifiable Data Partition

1. Run script `start-vd.sh` to start a root chain and 3 VD partition nodes.
2. Run script `stop-all.sh` to stop the root chain and partition nodes (or `stop-vd.sh` and `stop-root.sh`).

# User Token Partition
1. Run script `start-tokens.sh` to start a root chain and 3 Tokens partition nodes.
2. Run script `stop-all.sh` to stop the root chain and partition nodes (or `stop-tokens.sh` and `stop-root.sh`).

# Start all partitions at once
1. Run script `start-all.sh` to start money, vd and tokens nodes and a root chain.
2. Run `stop-all.sh` to stop everything

# Logging configuration

Logging can be configured through a yaml configuration file. See `cli/alphabill/config/logger-config.yaml` for example.

Default location of the logger configuration file is `$AB_HOME/logger-config.yaml`

The location can be changed through `--logger-config` configuration key. If it's relative URL, then it's relative
to `$AB_HOME`.

# Wallet Logging Configuration

Wallet logging can be configured only through CLI parameters. 

`./alphabill wallet --log-file=<path/to/my/file> --log-level=INFO`

Default log output is `stderr` and default log level is `INFO`. 

Possible log level values: `ERROR, WARNING, NOTICE, INFO, DEBUG`

# Set up autocompletion

To use autocompletion (supported with `bash`, `fish`, `powershell` and `zsh`), run the following commands after
building (this is `bash` example):

* `./alphabill completion bash > /tmp/completion`
* `source /tmp/completion`

# CI setup

See gitlab-ci.yml for details.

GitLab runs the CI job inside docker container defined in `alphabill/gitlab-ci-image`.
