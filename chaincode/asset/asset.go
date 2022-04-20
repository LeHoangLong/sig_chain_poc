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

type Material struct {
	graph.NodeHeader
	Name     string `json:"Name"`
	Unit     string `json:"Unit"`
	Quantity string `json:"Quantity"`
}

func (m *Material) GetHeader() graph.NodeHeader {
	return m.NodeHeader
}
func (m *Material) SetHeader(iHeader graph.NodeHeader) {
	m.NodeHeader = iHeader
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

func (c *MaterialContract) CreateMaterial(
	iCtx contractapi.TransactionContextInterface,
	iNodeId string,
	iName string,
	iUnit string,
	iQuantity string,
	iOwnerPublicKey string,
	iCreatedTime time.Time,
	iSignature string,
) error {
	quantity, err := decimal.NewFromString(iQuantity)
	if err != nil {
		return err
	}

	transactionTime, err := iCtx.GetStub().GetTxTimestamp()
	if err != nil {
		return err
	}

	timeDiff := transactionTime.Seconds - iCreatedTime.Unix()
	if timeDiff < 0 {
		timeDiff = -timeDiff
	}

	if timeDiff > 3600 {
		return fmt.Errorf("Timestamp does not match with transaction's timestamp")
	}

	graphContract := graph.GraphContract{}
	nodeHeader := graph.MakeNodeHeader(
		iNodeId,
		false,
		map[string]bool{},
		map[string]bool{},
		iOwnerPublicKey,
		iCreatedTime,
		iSignature,
	)
	material := MakeMaterial(
		iName,
		iUnit,
		quantity.String(),
		nodeHeader,
	)

	return graphContract.CreateNode(
		iCtx,
		&material,
	)
}

func MakeMaterial(
	iName string,
	iUnit string,
	iQuantity string,
	iHeader graph.NodeHeader,
) Material {
	return Material{
		NodeHeader: iHeader,
		Name:       iName,
		Unit:       iUnit,
		Quantity:   iQuantity,
	}
}

func (c *MaterialContract) GetMaterial(
	iCtx contractapi.TransactionContextInterface,
	iNodeId string,
) (*Material, error) {
	graphContract := graph.GraphContract{}
	var material Material
	err := graphContract.GetNode(iCtx, iNodeId, &material)
	if err != nil {
		return nil, err
	}

	return &material, nil
}

func (c *MaterialContract) TransferMaterial(
	iCtx contractapi.TransactionContextInterface,
	iNodeId string,
	iNewNodeId string,
	iNewOwnerPublicKey string,
	iSignature string,
	iNewNodeSignature string,
	iTransferTime time.Time,
) error {
	graphContract := graph.GraphContract{}

	var material Material
	err := graphContract.GetNode(iCtx, iNodeId, &material)
	if err != nil {
		return err
	}

	transactionTime, err := iCtx.GetStub().GetTxTimestamp()
	if err != nil {
		return err
	}

	timeDiff := transactionTime.Seconds - iTransferTime.Unix()
	if timeDiff < 0 {
		timeDiff = -timeDiff
	}

	if timeDiff > 3600 {
		return fmt.Errorf("Timestamp does not match with transaction's timestamp")
	}

	return graphContract.TransferNodeOwnership(
		iCtx,
		iNodeId,
		&material,
		iNewNodeId,
		iTransferTime,
		iNewOwnerPublicKey,
		iSignature,
		iNewNodeSignature,
	)
}

/// iSignature is the signature for the final finalized node
/// iNewNodeSignatures are the signatures for the new split nodes
/*
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
