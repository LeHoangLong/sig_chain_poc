/*
SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"log"
	"sig_chain/chaincode/asset"
	graph "sig_chain/chaincode/graph"

	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

func main() {
	assetChaincode, err := contractapi.NewChaincode(&graph.GraphContract{}, &asset.TypedGraphContract{}, &asset.MaterialContract{})
	if err != nil {
		log.Panicf("Error creating asset-transfer-basic chaincode: %v", err)
	}

	if err := assetChaincode.Start(); err != nil {
		log.Panicf("Error starting asset-transfer-basic chaincode: %v", err)
	}
}
