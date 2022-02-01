package graph

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha512"
	"crypto/x509"
	"encoding/json"
	"fmt"

	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

type BaseError struct {
	message string
}

func (m *BaseError) Error() string {
	return m.message
}

type NotFoundError struct {
	BaseError
}

type AlreadyExistsError struct {
	BaseError
}

// SmartContract provides functions for managing an Asset
type GraphContract struct {
	contractapi.Contract
}

/// Increment version after every update so that the next time an update is needed,
/// a different signature is needed
type Node struct {
	Id                    string          `json:"Id"`
	IsFinalized           bool            `json:"IsFinalized"`
	Data                  interface{}     `json:"Data"`
	NextNodeHashedIds     map[string]bool `json:"NextNodeHashedIds"` /// used a set
	PreviousNodeHashedIds map[string]bool `json:"PreviousNodeHashedIds"`
	OwnerPublicKey        string          `json:"OwnerPublicKey"`
}

func (n *Node) Verify(
	iSignature string,
) error {
	json, err := json.Marshal(n)
	if err != nil {
		return err
	}

	hash := sha512.Sum512(json)
	ifc, err := x509.ParsePKIXPublicKey([]byte(n.OwnerPublicKey))
	if err != nil {
		return err
	}
	key, ok := ifc.(*rsa.PublicKey)
	if !ok {
		return fmt.Errorf("unsupported key format")
	}
	err = rsa.VerifyPKCS1v15(key, crypto.SHA512, hash[:], []byte(iSignature))

	return err
}

func (n *Node) verifyWithPublicKey(
	iSignature string,
	iPublicKey string,
) error {
	newNodeJson, err := json.Marshal(n)
	if err != nil {
		return err
	}

	hash := sha512.Sum512(newNodeJson)
	ifc, err := x509.ParsePKIXPublicKey([]byte(iPublicKey))
	if err != nil {
		return err
	}
	key, ok := ifc.(*rsa.PublicKey)
	if !ok {
		return fmt.Errorf("unsupported key format")
	}
	err = rsa.VerifyPKCS1v15(key, crypto.SHA512, hash[:], []byte(iSignature))
	return err
}

func (c *GraphContract) GetNode(
	ctx contractapi.TransactionContextInterface,
	iNodeId string,
) (*Node, error) {
	nodeJson, err := ctx.GetStub().GetState(iNodeId)

	if err != nil {
		return nil, fmt.Errorf("could not get state with token id %s: %v", iNodeId, err)
	}

	if nodeJson == nil {
		return nil, &NotFoundError{
			BaseError{
				message: fmt.Sprintf("Token with id %s does not exist", iNodeId),
			},
		}
	}

	var node Node
	err = json.Unmarshal(nodeJson, &node)
	if err != nil {
		return nil, err
	}

	return &node, nil
}

func (c *GraphContract) FinalizeNode(
	ctx contractapi.TransactionContextInterface,
	iNodeId string,
	iSignature string,
) error {
	thisNode, err := c.GetNode(ctx, iNodeId)

	if err != nil {
		return err
	}

	if thisNode == nil {
		return &NotFoundError{
			BaseError{
				message: fmt.Sprintf("Node with id %s does not exist", iNodeId),
			},
		}
	}

	thisNode.IsFinalized = true

	err = thisNode.Verify(iSignature)

	if err != nil {
		return err
	}

	thisNodeJson, err := json.Marshal(thisNode)
	if err != nil {
		return err
	}

	return ctx.GetStub().PutState(iNodeId, thisNodeJson)
}

/// for batch update
func (c *GraphContract) UpdateNode(
	iCtx contractapi.TransactionContextInterface,
	iNode Node,
	iSignature string,
) error {
	oldNode, err := c.GetNode(iCtx, iNode.Id)
	if err != nil {
		return err
	}

	err = iNode.verifyWithPublicKey(
		iSignature,
		oldNode.OwnerPublicKey,
	)

	if err != nil {
		return err
	}

	newNodeJson, err := json.Marshal(iNode)
	if err != nil {
		return err
	}

	err = iCtx.GetStub().PutState(oldNode.Id, newNodeJson)
	return err
}

func (c *GraphContract) CreateEdge(
	ctx contractapi.TransactionContextInterface,
	iNodeId string,
	iNextNodeId string,
	iSignature string,
	iNextNodeSignature string,
) error {
	thisNode, err := c.GetNode(ctx, iNodeId)

	if err != nil {
		return err
	}

	if thisNode == nil {
		return &NotFoundError{
			BaseError{
				message: fmt.Sprintf("Node with id %s does not exist", iNodeId),
			},
		}
	}

	nextNode, err := c.GetNode(ctx, iNextNodeId)
	if err != nil {
		return err
	}

	if nextNode == nil {
		return &NotFoundError{
			BaseError{
				message: fmt.Sprintf("Node with id %s does not exist", iNextNodeId),
			},
		}
	}

	hasher := sha512.New()
	thisNode.NextNodeHashedIds[string(hasher.Sum([]byte(iNextNodeId)))] = true
	nextNode.PreviousNodeHashedIds[string(hasher.Sum([]byte(iNodeId)))] = true

	err = thisNode.Verify(iSignature)
	if err != nil {
		return err
	}

	err = nextNode.Verify(iNextNodeSignature)
	if err != nil {
		return err
	}

	thisNodeJson, err := json.Marshal(thisNode)
	if err != nil {
		return err
	}

	nextNodeJson, err := json.Marshal(nextNode)
	if err != nil {
		return err
	}

	err = ctx.GetStub().PutState(iNodeId, thisNodeJson)
	if err != nil {
		return err
	}

	err = ctx.GetStub().PutState(iNextNodeId, nextNodeJson)
	if err != nil {
		return err
	}

	return nil
}

/// new nodes reference to updated node
/// iSignature is to sign the updated node
func (c *GraphContract) CreateReferencedNodesAndFinalize(
	iCtx contractapi.TransactionContextInterface,
	iNodeId string,
	iNewNodeIds []string,
	iData []interface{},
	iOwnerPublicKey []string,
	iSignature string,
	iNewPublicKey []string,
) error {
	node, err := c.GetNode(iCtx, iNodeId)
	if err != nil {
		return err
	}

	if len(iNewNodeIds) != len(iData) {
		return fmt.Errorf("mistmach data and node ids")
	}

	if len(iData) != len(iOwnerPublicKey) {
		return fmt.Errorf("mistmach data and owner public key")
	}

	if node.IsFinalized {
		return fmt.Errorf("node finalized")
	}

	for _, id := range iNewNodeIds {
		idHash := sha512.Sum512([]byte(id))
		node.NextNodeHashedIds[string(idHash[:])] = true
	}
	node.IsFinalized = true

	nodeJson, err := json.Marshal(node)

	err = node.Verify(iSignature)
	if err != nil {
		return err
	}

	err = iCtx.GetStub().PutState(node.Id, nodeJson)
	if err != nil {
		return err
	}

	oldNodeHash := sha512.Sum512([]byte(node.Id))
	for i := 0; i < len(iData); i++ {
		nodeExists, err := c.DoesNodeExists(iCtx, iNewNodeIds[i])
		if err != nil {
			return err
		}

		if nodeExists {
			return fmt.Errorf("node already exists")
		}

		newNode := Node{
			Id:                    iNewNodeIds[i],
			IsFinalized:           false,
			Data:                  iData[i],
			NextNodeHashedIds:     map[string]bool{},
			PreviousNodeHashedIds: map[string]bool{string(oldNodeHash[:]): true},
			OwnerPublicKey:        iOwnerPublicKey[i],
		}

		newNodeJson, err := json.Marshal(newNode)
		if err != nil {
			return err
		}

		err = iCtx.GetStub().PutState(newNode.Id, newNodeJson)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *GraphContract) CreateNode(
	iCtx contractapi.TransactionContextInterface,
	iNodeId string,
	iData interface{},
	iOwnerPublicKey string,
	iSignature string,
) error {
	doesNodeExists, err := c.DoesNodeExists(iCtx, iNodeId)
	if err != nil {
		return err
	}

	if doesNodeExists {
		return &AlreadyExistsError{
			BaseError{
				message: "Node id already used",
			},
		}
	}

	ifc, err := x509.ParsePKIXPublicKey([]byte(iOwnerPublicKey))
	if err != nil {
		return err
	}

	if _, ok := ifc.(*rsa.PublicKey); !ok {
		return fmt.Errorf("unpported key format")
	}

	newNode := Node{
		Id:                    iNodeId,
		Data:                  iData,
		NextNodeHashedIds:     map[string]bool{},
		PreviousNodeHashedIds: map[string]bool{},
		OwnerPublicKey:        iOwnerPublicKey,
		IsFinalized:           false,
	}

	err = newNode.Verify(iSignature)
	if err != nil {
		return err
	}

	tokenJson, err := json.Marshal(newNode)
	if err != nil {
		return err
	}

	return iCtx.GetStub().PutState(iNodeId, tokenJson)
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

func (c *GraphContract) TransferNodeOwnership(
	iCtx contractapi.TransactionContextInterface,
	iNodeId string,
	iNewNodeId string,
	iNewOwnerPublicKey string,
	iSignature string,
	iNewNodeSignature string,
) error {
	oldNode, err := c.GetNode(iCtx, iNodeId)
	if err != nil {
		return err
	}

	nodeExists, err := c.DoesNodeExists(iCtx, iNewNodeId)
	if err != nil {
		return err
	}

	if nodeExists {
		return fmt.Errorf("node with id %s already exists", iNewNodeId)
	}

	newNode := oldNode
	newNode.OwnerPublicKey = iNewOwnerPublicKey

	hasher := sha512.New()

	oldNode.NextNodeHashedIds[string(hasher.Sum([]byte(iNewNodeId)))] = true
	oldNode.IsFinalized = true

	newNode.PreviousNodeHashedIds[string(hasher.Sum([]byte(iNodeId)))] = true

	err = oldNode.Verify(iSignature)
	if err != nil {
		return err
	}

	err = newNode.Verify(iSignature)
	if err != nil {
		return err
	}

	return nil
}
