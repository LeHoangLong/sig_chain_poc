package graph

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha512"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"time"

	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

// SmartContract provides functions for managing an Asset
type GraphContract struct {
}

/// Increment version after every update so that the next time an update is needed,
/// a different signature is needed
type NodeHeader struct {
	Id                    string          `json:"Id"`
	IsFinalized           bool            `json:"IsFinalized"`
	PreviousNodeHashedIds map[string]bool `json:"PreviousNodeHashedIds"` /// used as a set
	NextNodeHashedIds     map[string]bool `json:"NextNodeHashedIds"`     /// used as a set
	OwnerPublicKey        string          `json:"OwnerPublicKey"`
	CreatedTime           time.Time       `json:"CreatedTime"`
	Signature             string          `json:"Signature"`
}

type NodeI interface {
	GetHeader() NodeHeader
	SetHeader(NodeHeader)
}

func MakeNodeHeader(
	iId string,
	iIsFinalized bool,
	iPreviousNodeHashedIds map[string]bool,
	iNextNodeHashedIds map[string]bool,
	iOwnerPublicKey string,
	iCreatedTime time.Time,
	iSignature string,
) NodeHeader {
	return NodeHeader{
		Id:                    iId,
		IsFinalized:           iIsFinalized,
		NextNodeHashedIds:     iNextNodeHashedIds,
		PreviousNodeHashedIds: iPreviousNodeHashedIds,
		OwnerPublicKey:        iOwnerPublicKey,
		CreatedTime:           iCreatedTime,
		Signature:             iSignature,
	}
}

func parsePublicKey(
	iPublicKey string,
) (interface{}, error) {
	block, _ := pem.Decode([]byte(iPublicKey))
	return x509.ParsePKCS1PublicKey(block.Bytes)
}

func (c *GraphContract) Verify(
	iCtx contractapi.TransactionContextInterface,
	iSignature string,
	iNode NodeI,
) error {
	noSignatureHeader := iNode.GetHeader()
	originalHeader := iNode.GetHeader()
	noSignatureHeader.Signature = ""

	defer func() {
		iNode.SetHeader(originalHeader)
	}()
	iNode.SetHeader(noSignatureHeader)

	json, err := json.Marshal(iNode)
	fmt.Println("json: ", string(json))
	if err != nil {
		return err
	}

	hash := sha512.Sum512(json)
	ifc, err := parsePublicKey(iNode.GetHeader().OwnerPublicKey)
	if err != nil {
		return err
	}
	key, ok := ifc.(*rsa.PublicKey)
	if !ok {
		return fmt.Errorf("unsupported key format")
	}

	err = rsa.VerifyPKCS1v15(key, crypto.SHA512, hash[:], []byte(iSignature))
	if err != nil {
		return fmt.Errorf("verify err: %s", err.Error())
	}

	return err
}

func (c *GraphContract) GetNode(
	iCtx contractapi.TransactionContextInterface,
	iNodeId string,
	oNode interface{},
) error {
	nodeJson, err := iCtx.GetStub().GetState(iNodeId)

	if err != nil {
		return fmt.Errorf("could not get state with token id %s: %v", iNodeId, err)
	}

	if nodeJson == nil {
		return fmt.Errorf("Token with id %s does not exist", iNodeId)
	}

	err = json.Unmarshal(nodeJson, oNode)
	if err != nil {
		return err
	}

	return nil
}

/// iNode are used as placeholders for json unmarshal / marshal and can be empty
func (c *GraphContract) FinalizeNode(
	iCtx contractapi.TransactionContextInterface,
	iNodeId string,
	iSignature string,
	iNode NodeI,
) error {
	err := c.GetNode(iCtx, iNodeId, &iNode)

	if err != nil {
		return err
	}

	newHeader := iNode.GetHeader()
	newHeader.IsFinalized = true
	iNode.SetHeader(newHeader)

	err = c.Verify(iCtx, iSignature, iNode)

	if err != nil {
		return err
	}

	thisNodeJson, err := json.Marshal(iNode)
	if err != nil {
		return err
	}

	return iCtx.GetStub().PutState(iNodeId, thisNodeJson)
}

/// iNode and iNextNode are used as placeholders for json unmarshal / marshal and can be empty
func (c *GraphContract) CreateEdge(
	iCtx contractapi.TransactionContextInterface,
	iNodeId string,
	iNode NodeI,
	iNewSignature string,
	iNextNodeId string,
	iNextNode NodeI,
	iNextNodeNewSignature string,
) error {
	id := iNodeId
	nextNodeId := iNextNodeId
	err := c.GetNode(iCtx, id, &iNode)
	if err != nil {
		return err
	}
	if iNode.GetHeader().IsFinalized {
		return fmt.Errorf("node is already finalized")
	}

	err = c.GetNode(iCtx, nextNodeId, &iNextNode)
	if err != nil {
		return err
	}
	if iNextNode.GetHeader().IsFinalized {
		return fmt.Errorf("next node is already finalized")
	}

	hasher := sha512.New()
	iNode.GetHeader().NextNodeHashedIds[string(hasher.Sum([]byte(nextNodeId)))] = true
	iNextNode.GetHeader().PreviousNodeHashedIds[string(hasher.Sum([]byte(id)))] = true

	err = c.Verify(iCtx, iNewSignature, iNode)
	if err != nil {
		return err
	}

	err = c.Verify(iCtx, iNextNodeNewSignature, iNextNode)
	if err != nil {
		return err
	}

	thisNodeJson, err := json.Marshal(iNode)
	if err != nil {
		return err
	}

	nextNodeJson, err := json.Marshal(iNextNode)
	if err != nil {
		return err
	}

	err = iCtx.GetStub().PutState(id, thisNodeJson)
	if err != nil {
		return err
	}

	err = iCtx.GetStub().PutState(nextNodeId, nextNodeJson)
	if err != nil {
		return err
	}

	return nil
}

/// new nodes reference to updated node
/// iNewSignature is to sign the updated node
/// iNode is used as placeholders for json unmarshal / marshal and can be empty
func (c *GraphContract) CreateChildrenNodesAndFinalize(
	iCtx contractapi.TransactionContextInterface,
	iNodeId string,
	iNode NodeI,
	iNewSignature string,
	iChildren []NodeI,
) error {
	nodeId := iNodeId
	err := c.GetNode(iCtx, nodeId, &iNode)
	if err != nil {
		return err
	}

	header := iNode.GetHeader()

	if header.IsFinalized {
		return fmt.Errorf("node finalized")
	}

	for _, node := range iChildren {
		idHash := sha512.Sum512([]byte(node.GetHeader().Id))
		header.NextNodeHashedIds[string(idHash[:])] = true
	}
	header.IsFinalized = true

	err = c.Verify(iCtx, iNewSignature, iNode)
	if err != nil {
		return err
	}

	oldNodeHashBytes := sha512.Sum512([]byte(header.Id))
	oldNodeHash := string(oldNodeHashBytes[:])
	for _, child := range iChildren {
		nodeExists, err := c.DoesNodeExists(iCtx, child.GetHeader().Id)
		if err != nil {
			return err
		}

		if nodeExists {
			return fmt.Errorf("node already exists")
		}

		child.GetHeader().PreviousNodeHashedIds[oldNodeHash] = true

		err = c.Verify(iCtx, child.GetHeader().Signature, child)
		if err != nil {
			return err
		}

		newNodeJson, err := json.Marshal(child)
		if err != nil {
			return err
		}

		err = iCtx.GetStub().PutState(child.GetHeader().Id, newNodeJson)
		if err != nil {
			return err
		}
	}

	nodeJson, err := json.Marshal(iNode)
	err = iCtx.GetStub().PutState(header.Id, nodeJson)
	if err != nil {
		return err
	}

	return nil
}

func (c *GraphContract) CreateNode(
	iCtx contractapi.TransactionContextInterface,
	iNode NodeI,
) error {
	doesNodeExists, err := c.DoesNodeExists(iCtx, iNode.GetHeader().Id)
	if err != nil {
		return err
	}

	if doesNodeExists {
		return fmt.Errorf("Node id already used")
	}

	err = c.Verify(iCtx, iNode.GetHeader().Signature, iNode)
	fmt.Printf("iNode: %+v\n", iNode)
	if err != nil {
		return err
	}

	nodeJson, err := json.Marshal(iNode)
	if err != nil {
		return err
	}

	return iCtx.GetStub().PutState(iNode.GetHeader().Id, nodeJson)
}

func (c *GraphContract) DoesNodeExists(
	iCtx contractapi.TransactionContextInterface,
	iNodeId string,
) (bool, error) {
	nodeJson, err := iCtx.GetStub().GetState(iNodeId)

	if err != nil {
		return false, fmt.Errorf("failed to read from ledger: %v", err)
	}

	return nodeJson != nil, nil
}

func (c *GraphContract) AreIdsAvailable(
	iCtx contractapi.TransactionContextInterface,
	iIds []string,
) ([]bool, error) {
	ret := []bool{}
	for _, id := range iIds {
		nodeJson, err := iCtx.GetStub().GetState(id)
		if err != nil {
			return []bool{}, fmt.Errorf("failed to read from ledger: %v", err)
		}

		ret = append(ret, nodeJson == nil)
	}

	return ret, nil
}

func (c *GraphContract) TransferNodeOwnership(
	iCtx contractapi.TransactionContextInterface,
	iNodeId string,
	iNode NodeI,
	iNewNodeId string,
	iTransferTime time.Time,
	iNewOwnerPublicKey string,
	iNewSignature string,
	iNewNodeSignature string,
) error {
	id := iNodeId
	nodeExists, err := c.DoesNodeExists(iCtx, id)
	if err != nil {
		return err
	}
	if !nodeExists {
		return fmt.Errorf("node with id %s does not exists", id)
	}

	nodeExists, err = c.DoesNodeExists(iCtx, iNewNodeId)
	if err != nil {
		return err
	}
	if nodeExists {
		return fmt.Errorf("node with id %s already exists", iNewNodeId)
	}

	newNode := iNode
	newHeader := newNode.GetHeader()
	newHeader.Id = iNewNodeId
	newHeader.OwnerPublicKey = iNewOwnerPublicKey
	newHeader.CreatedTime = iTransferTime
	newNode.SetHeader(newHeader)

	hasher := sha512.New()

	oldNode := iNode
	oldNodeHeader := iNode.GetHeader()
	oldNodeHeader.NextNodeHashedIds[string(hasher.Sum([]byte(iNewNodeId)))] = true
	oldNodeHeader.IsFinalized = true
	oldNode.SetHeader(oldNodeHeader)

	newNode.GetHeader().PreviousNodeHashedIds[string(hasher.Sum([]byte(id)))] = true

	err = c.Verify(iCtx, iNewSignature, oldNode)
	if err != nil {
		return err
	}

	err = c.Verify(iCtx, iNewNodeSignature, newNode)
	if err != nil {
		return err
	}

	nodeJson, err := json.Marshal(oldNode)
	if err != nil {
		return err
	}
	err = iCtx.GetStub().PutState(id, nodeJson)
	if err != nil {
		return err
	}

	nodeJson, err = json.Marshal(newNode)
	if err != nil {
		return err
	}
	err = iCtx.GetStub().PutState(iNewNodeId, nodeJson)
	if err != nil {
		return err
	}

	return nil
}
