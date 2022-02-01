package asset

import (
	"fmt"
	"sig_chain/chaincode/graph"
	"time"

	"github.com/hyperledger/fabric-contract-api-go/contractapi"
	"github.com/shopspring/decimal"
)

type NodeType = string

const (
	eMaterial             NodeType = "eMaterial"
	eCertificate          NodeType = "eCertificate"
	eCertificateAuthority NodeType = "eCertificateAuthority"
	eData                 NodeType = "eData"
	eHash                 NodeType = "eHash"
)

type TypedNode struct {
	Type string      `json:"Type"` /// Material / Certificate / CertificateAuthority / Data / Hash
	Data interface{} `json:"Data"`
}

type Material struct {
	Name     string          `json:"Name"`
	Unit     string          `json:"Unit"`
	Quantity decimal.Decimal `json:"Quantity"`
}

type CertificateAuthority struct {
	RevokedCertificateIds []string `json:"RevokedCertificateIds"`
	RootId                string   `json:"RootId"` /// Easier to trace since the node only stores hash of the issuer
}

type Certificate struct {
	IssueTime  time.Time `json:"IssueTime"`
	ExpiryTime time.Time `json:"ExpiryTime"`
	Signature  string    `json:"Signature"`
	IssuerId   string    `json:"IssuerId"` /// Easier to trace since the node only stores hash of the issuer
}

type MaterialContract struct {
	contractapi.Contract
}

type TypedGraphContract struct {
	contractapi.Contract
}

func (c *TypedGraphContract) CreateReferencedTypedNodesAndFinalize(
	iCtx contractapi.TransactionContextInterface,
	iNodeId string,
	iNewNodeIds []string,
	iType []string,
	iData []interface{},
	iOwnerPublicKey []string,
	iSignature string,
	iNewNodeSignatures []string,
) error {
	typedNodes := []interface{}{}

	if len(iNewNodeIds) != len(iType) {
		return fmt.Errorf("mismatch new node id and types")
	}

	if len(iNewNodeIds) != len(iData) {
		return fmt.Errorf("mismatch new node id and data")
	}

	if len(iNewNodeIds) != len(iOwnerPublicKey) {
		return fmt.Errorf("mismatch new node id and owner public key")
	}

	for i, data := range iData {
		typedNode := TypedNode{
			Data: data,
			Type: iType[i],
		}

		typedNodes = append(typedNodes, typedNode)
	}

	graphContract := graph.GraphContract{}
	err := graphContract.CreateReferencedNodesAndFinalize(
		iCtx,
		iNodeId,
		iNewNodeIds,
		typedNodes,
		iOwnerPublicKey,
		iSignature,
		iNewNodeSignatures,
	)
	return err
}

func (c *TypedGraphContract) CreateTypedNode(
	iCtx contractapi.TransactionContextInterface,
	iNodeId string,
	iType string,
	iData interface{},
	iOwnerPublicKey string,
	iSignature string,
) error {
	graphContract := graph.GraphContract{}
	typedNode := TypedNode{
		Data: iData,
		Type: iType,
	}

	return graphContract.CreateNode(
		iCtx,
		iNodeId,
		typedNode,
		iOwnerPublicKey,
		iSignature,
	)
}

func (c *TypedGraphContract) GetTypedNode(
	iCtx contractapi.TransactionContextInterface,
	iNodeId string,
) (*TypedNode, error) {
	graphContract := graph.GraphContract{}
	node, err := graphContract.GetNode(iCtx, iNodeId)
	if err != nil {
		return nil, err
	}

	if typedNode, ok := node.Data.(TypedNode); ok {
		return &typedNode, err
	} else {
		return nil, fmt.Errorf("invalid node date")
	}
}

func (c *MaterialContract) CreateMaterial(
	iCtx contractapi.TransactionContextInterface,
	iNodeId string,
	iName string,
	iUnit string,
	iQuantity decimal.Decimal,
	iOwnerPublicKey string,
	iSignature string,
) error {
	graphContract := TypedGraphContract{}
	material := Material{
		Name:     iName,
		Unit:     iUnit,
		Quantity: iQuantity,
	}

	return graphContract.CreateTypedNode(
		iCtx,
		iNodeId,
		eMaterial,
		material,
		iOwnerPublicKey,
		iSignature,
	)
}

func (c *MaterialContract) GetMaterial(
	iCtx contractapi.TransactionContextInterface,
	iNodeId string,
) (*Material, error) {
	typedGraphContract := TypedGraphContract{}
	typedNode, err := typedGraphContract.GetTypedNode(iCtx, iNodeId)
	if err != nil {
		return nil, err
	}

	if typedNode.Type != eMaterial {
		return nil, fmt.Errorf("incorrect type")
	}

	if material, ok := typedNode.Data.(Material); ok {
		return &material, nil
	} else {
		return nil, fmt.Errorf("not a material")
	}
}

func (c *MaterialContract) TransferMaterial(
	iCtx contractapi.TransactionContextInterface,
	iNodeId string,
	iNewNodeId string,
	iNewOwnerPublicKey string,
	iSignature string,
	iNewNodeSignature string,
) error {
	graphContract := graph.GraphContract{}
	return graphContract.TransferNodeOwnership(
		iCtx,
		iNodeId,
		iNewNodeId,
		iNewOwnerPublicKey,
		iSignature,
		iNewNodeSignature,
	)
}

/// iSignature is the signature for the final finalized node
/// iNewNodeSignatures are the signatures for the new split nodes
func (c *MaterialContract) SplitMaterial(
	iCtx contractapi.TransactionContextInterface,
	iNodeId string,
	iSplitQuantities []decimal.Decimal,
	iNewNodeIds []string,
	iNewNodeOwnerPublicKeys []string,
	iSignature string,
	iNewNodeSignatures []string,
) error {
	if len(iSplitQuantities) != len(iNewNodeIds) {
		return fmt.Errorf("mismatch new node ids and split quantities")
	}

	if len(iSplitQuantities) != len(iNewNodeOwnerPublicKeys) {
		return fmt.Errorf("mismatch owner public keys and split quantities")
	}

	parentMaterial, err := c.GetMaterial(iCtx, iNodeId)
	if err != nil {
		return err
	}

	var total decimal.Decimal
	for _, quantity := range iSplitQuantities {
		total = total.Add(quantity)
	}

	if !total.Equals(parentMaterial.Quantity) {
		return fmt.Errorf("incorrect quantities")
	}

	if len(iSplitQuantities) == 0 {
		return fmt.Errorf("cannot have empty split quantities")
	}

	splitMaterial := []interface{}{}
	nodeType := []string{}
	for _, quantity := range iSplitQuantities {
		material := Material{
			Name:     parentMaterial.Name,
			Unit:     parentMaterial.Unit,
			Quantity: quantity,
		}
		splitMaterial = append(splitMaterial, material)
		nodeType = append(nodeType, eMaterial)
	}

	typedGraphContract := TypedGraphContract{}
	err = typedGraphContract.CreateReferencedTypedNodesAndFinalize(
		iCtx,
		iNodeId,
		iNewNodeIds,
		nodeType,
		splitMaterial,
		iNewNodeOwnerPublicKeys,
		iSignature,
		iNewNodeSignatures,
	)
	return err
}

/// TODO: Add support for merge and consuming materials
/*
func (c *MaterialContract) MergeMaterials(
	iCtx contractapi.TransactionContextInterface,
	iNodeIds []string,
	iSignatures []string,
	iNewNodeId string,
	iNewOwnerPublicKey string,
) error {
	if len(iNodeIds) == 0 {
		return fmt.Errorf("input node ids cannot be empty")
	}
	unit := ""
	name := ""
	quantity := decimal.NewFromInt(0)

	if len(iNodeIds) != len(iSignatures) {
		return fmt.Errorf("mismatch node ids and signatures")
	}

	for _, nodeId := range iNodeIds {
		material, err := c.GetMaterial(iCtx, nodeId)
		if err != nil {
			return err
		}

		if unit != "" && material.Unit != unit {
			return fmt.Errorf("Materials must have same unit")
		}

		if name != "" && material.Name != name {
			return fmt.Errorf("Materials must have same name")
		}
		unit = material.Unit
		name = material.Name
		quantity = quantity.Add(material.Quantity)
	}

	err := c.CreateMaterial(
		iCtx,
		iNewNodeId,
		name,
		unit,
		quantity,
		iNewOwnerPublicKey,
	)
	if err != nil {
		return err
	}

	return err
}
*/
