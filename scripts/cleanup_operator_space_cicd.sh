#!/bin/bash

SPACE_MRN=$(mondoo --config ./creds.json status -o json 2>/dev/null | jq '.agent.spaceMrn' -r)
if [[ $SPACE_MRN == "" ]]
then
	echo "Couldn't fetch spaceMrn from Mondoo status!"
	exit 1
fi

TOKEN=$(mondoo --config ./creds.json auth generate-api-access-token 2>&1 | grep Bearer | tr -d "[]")
if [[ $TOKEN == "" ]]
then
	echo "Couldn't get API token!"
        exit 1
fi

API_ENDPOINT=$(mondoo --config ./creds.json status -o json 2>/dev/null | jq '.api.endpoint' -r)
if [[ $API_ENDPOINT == "" ]]
then
	echo "Couldn't get API endpoint!"
	exit 1
fi

ASSET_QUERY='{
    "operationName":"LoadCicdProjects",
    "query":"query LoadCicdProjects($input: CicdProjectsInput!) {\n  cicdProjects(input: $input) {\n    ... on CicdProjects {\n      projects {\n        totalCount\n        edges {\n                    
    node {\n            mrn\n            }\n          
    }\n        pageInfo {\n          startCursor\n          endCursor\n          hasNextPage\n          hasPreviousPage\n          __typename\n        }\n        __typename\n      }\n      __typename\n    }\n    __typename\n  }\n}\n",'

ASSET_QUERY="$ASSET_QUERY
    \"variables\":
    {
      \"input\":
      {
        \"spaceMrn\":\"$SPACE_MRN\"
      }
    }
}
"    

echo "#Total Projects:"
/usr/bin/curl -s -X POST -H "Content-Type: application/json" -H "authorization: $TOKEN" --data @/tmp/mondoo_asset_query_cicd.json $API_ENDPOINT/query | jq '.data.cicdProjects.projects.totalCount'
MRNS=$(/usr/bin/curl -s -X POST -H "Content-Type: application/json" -H "authorization: $TOKEN" --data @/tmp/mondoo_asset_query_cicd.json $API_ENDPOINT/query | jq '.data.cicdProjects.projects.edges[].node.mrn' -r | xargs -I{} echo "\"{}\"§" | tr -d "\n")


DELETE_QUERY_STATIC='{"operationName":"DeleteCicdProjects","query":"mutation DeleteCicdProjects($input: DeleteProjectsInput!) {\n  deleteCicdProjects(input: $input) {\n    mrns\n    __typename\n  }\n}\n",'

export IFS="§"
MRN_BATCH=""
LOOP_INDEX=0
for mrn_to_delete in $MRNS; do
  LOOP_INDEX=$(($LOOP_INDEX+1))
  MRN_BATCH="${MRN_BATCH}${mrn_to_delete},"
  if [[ $(($LOOP_INDEX % 11)) == 0 ]]
  then
	DELETE_QUERY="$DELETE_QUERY_STATIC
		\"variables\":{
	  	  \"input\":{
	    	    \"mrns\":[${MRN_BATCH%?}]
          	  }
        	}
        }"
        echo $DELETE_QUERY > /tmp/mondoo_delete_query_cicd.json
        /usr/bin/curl -s -X POST -H "Content-Type: application/json" -H "authorization: $TOKEN" --data @/tmp/mondoo_delete_query_cicd.json $API_ENDPOINT/query | jq
        MRN_BATCH=""
        LOOP_INDEX=0
  fi
done
# delete rest after the loop

DELETE_QUERY="$DELETE_QUERY_STATIC
	\"variables\":{
  	  \"input\":{
    	    \"mrns\":[${MRN_BATCH%?}]
       	  }
       	}
}"


echo $DELETE_QUERY > /tmp/mondoo_delete_query_cicd.json
/usr/bin/curl -s -X POST -H "Content-Type: application/json" -H "authorization: $TOKEN" --data @/tmp/mondoo_delete_query_cicd.json $API_ENDPOINT/query | jq
