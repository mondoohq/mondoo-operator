#!/bin/bash

echo $MONDOO_CONFIG_PATH
ls -l $MONDOO_CONFIG_PATH

SPACE_MRN=$(jq '.space_mrn' -r  ./creds_editor.json)
if [[ $SPACE_MRN == "" ]]
then
	echo "Couldn't fetch spaceMrn from Mondoo status!"
	exit 1
fi

TOKEN=$(mondoo --config ./creds_editor.json auth generate-api-access-token 2>&1 | grep Bearer | tr -d "[]")
if [[ $TOKEN == "" ]]
then
	echo "Couldn't get API token!"
	exit 1
fi

API_ENDPOINT=$(jq '.api_endpoint' -r ./creds_editor.json)
if [[ $API_ENDPOINT == "" ]]
then
	echo "Couldn't get API endpoint!"
	exit 1
fi

ASSET_QUERY='{
    "operationName":"AssetForwardPagination",
    "query":"query AssetForwardPagination($spaceMrn: String!, $after: String, $first: Int, $queryTerms: [String!], $platformTitle: [String!], $platformName: [String!], $platformKind: [PlatformKind!], $platformRuntime: [String!], $scoreRange: [ScoreRange!], $scoreType: ScoreType!, $labels: [KeyValueInput!], $updated: AssetUpdateFilter, $eol: AssetEolFilter, $reboot: AssetOSRebootFilter, $exploitable: AssetExploitableFilter, $orderBy: AssetOrder) {\n  assets(\n    spaceMrn: $spaceMrn\n    after: $after\n    first: $first\n    orderBy: $orderBy\n    queryTerms: $queryTerms\n    platformTitle: $platformTitle\n    platformName: $platformName\n    platformKind: $platformKind\n    platformRuntime: $platformRuntime\n    scoreRange: $scoreRange\n    scoreType: $scoreType\n    labels: $labels\n    updated: $updated\n    eol: $eol\n    reboot: $reboot\n    exploitable: $exploitable\n  ) {\n    ...AssetFields\n    __typename\n  }\n}\n\nfragment AssetFields on AssetsConnection {\n  totalCount\n  edges {\n    node {\n    mrn\n    }\n      }\n  pageInfo {\n    startCursor\n    endCursor\n    hasNextPage\n    __typename\n  }\n  __typename\n}\n",'
ASSET_QUERY="$ASSET_QUERY
    \"variables\":
    {
        \"spaceMrn\":\"$SPACE_MRN\",
        \"scoreRange\":[],
        \"scoreType\":\"UNKNOWN\",
        \"platformName\":[],
        \"platformKind\":[],
        \"platformRuntime\":[],
        \"labels\":[],
        \"eol\":null,
        \"reboot\":null,
        \"exploitable\":null,
        \"updated\":null,
        \"orderBy\":
        {
          \"field\":\"LAST_UPDATED\",
          \"direction\":\"DESC\"
        },
        \"queryTerms\":[],
        \"first\":25
    }
}"
echo $ASSET_QUERY > /tmp/mondoo_asset_query.json

echo "Get MRNs"
MRNS=$(/usr/bin/curl -s -X POST -H "Content-Type: application/json" -H "authorization: $TOKEN" --data @/tmp/mondoo_asset_query.json $API_ENDPOINT/query | jq '.data.assets.edges[].node.mrn' -r | xargs -I{} echo "\"{}\"," | tr -d "\n")

DELETE_QUERY_STATIC='{
    "operationName":"DeleteAssets",
    "query":"mutation DeleteAssets($input: DeleteAssetsInput) {\n  deleteAssets(input: $input) {\n    assetMrns\n    errors\n    __typename\n  }\n}\n",'
#echo /usr/bin/curl -s -X POST -H "Content-Type: application/json" -H "authorization: $TOKEN" --data @/tmp/mondoo_asset_query.json $API_ENDPOINT/query
#/usr/bin/curl -s -X POST -H "Content-Type: application/json" -H "authorization: $TOKEN" --data @/tmp/mondoo_asset_query.json $API_ENDPOINT/query | jq
while [ true ]
do
  MRNS=$(/usr/bin/curl -s -X POST -H "Content-Type: application/json" -H "authorization: $TOKEN" --data @/tmp/mondoo_asset_query.json $API_ENDPOINT/query | jq '.data.assets.edges[].node.mrn' -r | xargs -I{} echo "\"{}\"," | tr -d "\n")
  if [[ ${#MRNS} == 0 ]]
  then
    echo "All assets deleted"
    break
  fi

  echo "Going to delete these assets:"
  echo $MRNS

  DELETE_QUERY="$DELETE_QUERY_STATIC
    \"variables\":
    {
        \"input\":
        {
            \"spaceMrn\":\"$SPACE_MRN\",
            \"assetMrns\":
            [
                ${MRNS%?}
            ]
        }
    }
  }"

  echo $DELETE_QUERY > /tmp/mondoo_delete_query.json
  #$CURL -s --data "$(echo $DELETE_QUERY)" $API_ENDPOINT/query | jq
  /usr/bin/curl -s -X POST -H "Content-Type: application/json" -H "authorization: $TOKEN" --data @/tmp/mondoo_delete_query.json $API_ENDPOINT/query | jq
  sleep 1
done