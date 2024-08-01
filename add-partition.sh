echo "1. Setup network with DRC and Tokens Partition..."
# ensure network is not running
bash stop.sh -a
# clean build dir
rm -rf testab
mkdir testab
# build genesis files for 3 root and 3 tokens nodes (compiling step was removed from setup-testab.sh)
bash setup-testab.sh -r 3 -t 3 -e 0 -o 0 -m 0

echo "starting the network..."
bash start.sh -r -p tokens
sleep 5 # let the network run a bit

echo "2. Generate genesis for permissioned tokens partition"
source helper.sh # get common functions
enterpriseTokensSdr='{"system_identifier": 5, "t2timeout": 2500}'
echo "$enterpriseTokensSdr" >testab/tokens-sdr-sid-5.json
generate_partition_node_genesis "tokens-enterprise" "3" "--system-identifier 5 --admin-key 028834d671a927762584091403259bff4bc972c917c7de8eb558118fabf9733384"
generate_log_configuration "testab/tokens_enterprise*/"

echo "3. Add permissioned tokens nodes' identity into RC configuration"
generate_root_genesis 3

echo "4. Upload new root genesis to RC"
sleep 5 # wait a bit before uploading conf
# have to upload configuration to every RC node
curl -X PUT -H "Content-Type: application/json" -d @testab/rootchain1/rootchain/root-genesis.json "http://localhost:27662/api/v1/configurations?start-round=20"
curl -X PUT -H "Content-Type: application/json" -d @testab/rootchain2/rootchain/root-genesis.json "http://localhost:27663/api/v1/configurations?start-round=20"
curl -X PUT -H "Content-Type: application/json" -d @testab/rootchain3/rootchain/root-genesis.json "http://localhost:27664/api/v1/configurations?start-round=20"
sleep 5 # wait a bit after uploading conf

echo "5. Start enterprise tokens partition"
bash start.sh -p tokens-enterprise
