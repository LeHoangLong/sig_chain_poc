package token_hyperledger

import (
	"encoding/hex"
	"encoding/json"
	"fmt"

	"crypto/sha512"

	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

// SmartContract provides functions for managing an Asset
type TokenContract struct {
	contractapi.Contract
}

type Token struct {
	Id                 string   `json:"Id"`
	ConsumingTokenId   string   `json:"ConsumingTokenId"`
	ConsumedTokenIds   []string `json:"ConsumedTokenIds"`
	RequestToAcceptUrl string   `json:"RequestToAcceptUrl"`
	RequestToSendUrl   string   `json:"RequestToSendUrl"`
	OwnerPublicKey     string   `json:"OwnerPublicKey"`
}

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

type TokenAlreadyConsumed struct {
	BaseError
}

const AlwaysAcceptUrl string = "sc://always-accept"
const AlwaysSendUrl string = "sc://always-send"

func (c *TokenContract) ConsumeToken(
	ctx contractapi.TransactionContextInterface,
	consumedTokenId string,
	consumingTokenId string,
) error {
	consumedToken, err := c.GetToken(ctx, consumedTokenId)

	if err != nil {
		return err
	}

	if consumedToken == nil {
		return &NotFoundError{
			BaseError{
				message: fmt.Sprintf("To be consumed token with id %s does not exist", consumedTokenId),
			},
		}
	}

	if consumedToken.ConsumingTokenId != "" {
		return &TokenAlreadyConsumed{
			BaseError{
				message: fmt.Sprintf("To be consumed token with id %s already consumed", consumedTokenId),
			},
		}
	}

	consumingToken, err := c.GetToken(ctx, consumingTokenId)
	if err != nil {
		return err
	}

	if consumingToken == nil {
		return &NotFoundError{
			BaseError{
				message: fmt.Sprintf("Consuming token with id %s does not exist", consumingTokenId),
			},
		}
	}

	if consumedToken.RequestToSendUrl != "" && consumingToken.RequestToAcceptUrl != "" {
		fmt.Println("Consuming token")
		consumedToken.ConsumingTokenId = consumingTokenId
		consumingToken.ConsumedTokenIds = append(consumingToken.ConsumedTokenIds, consumedTokenId)
		// TODO: verify by calling request to send and request to accept
		// Each calls will send a post request with a random challenge and then verify
		// by using the tokenId
		consumedTokenJson, err := json.Marshal(consumedToken)
		if err != nil {
			return err
		}

		consumingTokenJson, err := json.Marshal(consumingToken)
		if err != nil {
			return err
		}

		ctx.GetStub().PutState(consumedTokenId, consumedTokenJson)
		ctx.GetStub().PutState(consumingTokenId, consumingTokenJson)
	}

	return nil
}

func (c *TokenContract) GetToken(
	ctx contractapi.TransactionContextInterface,
	tokenId string,
) (*Token, error) {
	tokenJson, err := ctx.GetStub().GetState(tokenId)

	if err != nil {
		return nil, fmt.Errorf("Could not get state with token id %s: %v", tokenId, err)
	}

	if tokenJson == nil {
		return nil, &NotFoundError{
			BaseError{
				message: fmt.Sprintf("Token with id %s does not exist", tokenId),
			},
		}
	}

	var token Token
	err = json.Unmarshal(tokenJson, &token)
	if err != nil {
		return nil, err
	}

	return &token, nil

}

func (c *TokenContract) CreateToken(
	ctx contractapi.TransactionContextInterface,
	seed string,
	requestToAcceptUrl string,
	requestToSendUrl string,
	ownerPublicKey string,
) error {
	tokenIdBytes := sha512.Sum512([]byte(seed))
	tokenId := hex.EncodeToString(tokenIdBytes[:])
	doesTokenExist, err := c.DoesTokenExist(ctx, tokenId)

	if err != nil {
		return err
	}

	if doesTokenExist {
		return &AlreadyExistsError{
			BaseError{
				message: "Address already used",
			},
		}
	}

	newToken := Token{
		Id:                 tokenId,
		ConsumingTokenId:   "",
		ConsumedTokenIds:   []string{},
		RequestToAcceptUrl: requestToAcceptUrl,
		RequestToSendUrl:   requestToSendUrl,
		OwnerPublicKey:     ownerPublicKey,
	}

	tokenJson, err := json.Marshal(newToken)
	if err != nil {
		return err
	}

	return ctx.GetStub().PutState(tokenId, tokenJson)
}

func (c *TokenContract) DoesTokenExist(
	ctx contractapi.TransactionContextInterface,
	tokenId string,
) (bool, error) {
	tokenJson, err := ctx.GetStub().GetState(tokenId)

	if err != nil {
		return false, fmt.Errorf("Failed to read from ledger: %v", err)
	}

	return tokenJson != nil, nil
}
