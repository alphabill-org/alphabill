job "[[ .gt_environment ]]-ab-money" {
    datacenters = ["[[ .nomad_datacenter ]]"]
    type = "service"

    constraint {
        attribute = "${meta.group}"
        value = "apps_money_partition"
    }

    group "app" {
        constraint {
            operator  = "distinct_hosts"
            value     = "true"
        }

        count = [[ len .ab_money_partition_ips ]]

        network {
            port "raft" { static = [[ .ab_money_partition_raft_port ]] }
            port "grpc" { static = [[ .ab_money_partition_port ]] }
        }

        service {
            port = "raft"
            check {
                type = "tcp"
                interval = "10s"
                timeout  = "2s"
            }
        }

        service {
            port = "grpc"
            check {
                type = "grpc"
                interval = "10s"
                timeout  = "2s"
            }
        }

        task "ab-money" {
            driver = "exec"

            artifact {
                source = "http://nexus.[[ .gt_domain ]]:8081/repository/binary-raw/alphabill/[[ .alphabill_version ]].tar.gz"
                destination = "local/alphabill"
                mode = "file"
            }

            config {
                command = "local/alphabill"
                args = [
                    "money",
                    "--address", "/ip4/${NOMAD_IP_raft}/tcp/${NOMAD_PORT_raft}",
                    "--genesis", "/local/partition-genesis.json",
                    "--key-file", "/secrets/keys.json",
                    "--peers", "${PEERS}",
                    "--rootchain", "/ip4/[[ .ab_rootchain_ip ]]/tcp/[[ .ab_rootchain_port ]]",
                    "--server-address", ":${NOMAD_PORT_grpc}",
                    "--home", "${NOMAD_TASK_DIR}",
                ]
            }

            env {
                GENESIS_PATH = "[[ .gt_environment ]]/money-rootchain/rootchain/partition-genesis-0.json"
                KEYS_PATH = "[[ .gt_environment ]]/money${meta.index}/money/keys.json"
            }

            template {
                data = "{{ key (env \"GENESIS_PATH\") }}"
                destination = "local/partition-genesis.json"
            }

            template {
                data = "{{ key (env \"KEYS_PATH\") }}"
                destination = "secrets/keys.json"
            }

            template {
                data = "PEERS={{ key \"[[ .gt_environment ]]/peers.txt\" }}"
                destination = "local/peers.env"
                env = "true"
            }

            [[ indent 12 (fileContents "jobs/common/logger-config.hcl") ]]

            resources {
                cpu     = 3500
                memory  = 3500
            }
        }

[[ indent 8 (fileContents "jobs/common/restart.hcl") ]]
    }
}
