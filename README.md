# Build

Run `make build` to build the application. Executable will be built to `build/alphabill`. 

### Build dependencies

* `golang` version 1.21. (https://go.dev/doc/install)
* in order to rebuild everything including protobuf definitions (`make` or `make all`):
  * `protoc` version 3.21.9+ (https://grpc.io/docs/protoc-installation)
  * `protoc-gen-go` (https://grpc.io/docs/languages/go/quickstart/)

# Money Partition

1. Run script `./setup-testab.sh -m 3 -t 0 -e 0` to generate configuration for a root chain and 3 money partition nodes.
    The script generates rootchain and partition node keys, genesis files.
    Node configuration files are located in `testab` directory.
2. Run script `./start.sh -r -p money -b money` to start rootchain and 3 money partition nodes and money backend

3. Run script `stop.sh -a` to stop the root chain and partition nodes.
   
   Alternatively, use `stop.sh` to stop any partition or root and `start.sh` to resume. See command help for more details. 

## Configuration

It's possible to define the configuration values from (in the order of precedence):

* Command line flags (e.g. `--address="/ip4/127.0.0.1/tcp/26652"`)
* Environment (Prefix 'AB' must be used. E.g. `AB_ADDRESS="/ip4/127.0.0.1/tcp/26652"`)
* Configuration file (properties file) (E.g. `address="/ip4/127.0.0.1/tcp/26652"`)
* Default values

The default location of configuration file is `$AB_HOME/config.props`

The default `$AB_HOME` is `$HOME/.alphabill`

# User Token Partition
Typical set-up would run money and user token partition as fee credits need to be added to the user token partition
in order to pay for transactions.
Theoretically it is also possible run only the user token partition on its own, but it would not make much sense.
1. Run script `./setup-testab.sh -m 3 -t 3 -e 0` to generate configuration for a root chain and 3 money and token partition nodes.
   The script generates rootchain and partition node keys, genesis files.
   Node configuration files are located in `testab` directory.
2. Run script `./start.sh -r -p money -p tokens -b money -b tokens` to start rootchain and 3 partition nodes (money and token) and backends (money and token)
3. Run script `stop.sh -a` to stop the root chain and partition nodes.

# Evm Partition
Typical set-up would run money and evm partition as fee credits need to be added to the evm partition
in order to create an account and pay for transactions.
Theoretically it is also possible run only the evm partition on its own, but it would not make much sense.
1. Run script `./setup-testab.sh -m 3 -t 0 -e 3` to generate configuration for a root chain and 3 money and evm partition nodes.
   The script generates rootchain and partition node keys, genesis files.
   Node configuration files are located in `testab` directory.
2. Run script `./start.sh -r -p money -p evm -b money` to start rootchain, partition nodes (evm, money) and money backend
3. Run script `stop.sh -a` to stop the root chain and partition nodes.


# Start all partitions at once
1. Run script `./setup-testab.sh` to generate genesis for root, and 3 money, tokens and evm nodes.
2. Run `start.sh -r -p money -p tokens -p evm -b money -b tokens` to start everything
3. Run `stop.sh -a` to stop everything

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
