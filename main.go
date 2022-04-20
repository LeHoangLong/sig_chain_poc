/*
SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"log"
	"sig_chain/chaincode/asset"

	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

func main() {
	assetChaincode, err := contractapi.NewChaincode(
		&asset.MaterialContract{},
	)
	if err != nil {
		log.Panicf("Error creating asset-transfer-basic chaincode: %v", err)
	}

	if err := assetChaincode.Start(); err != nil {
		log.Panicf("Error starting asset-transfer-basic chaincode: %v", err)
	}
}
