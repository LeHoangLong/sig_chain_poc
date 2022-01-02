package token_hyperledger

import (
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/hyperledger/fabric-chaincode-go/shim"
	"github.com/hyperledger/fabric-contract-api-go/contractapi"
	"github.com/hyperledger/fabric-samples/asset-transfer-basic/chaincode-go/chaincode/mocks"
	"github.com/stretchr/testify/require"
)

//go:generate counterfeiter -o mocks/transaction.go -fake-name TransactionContext . transactionContext
type transactionContext interface {
	contractapi.TransactionContextInterface
}

//go:generate counterfeiter -o mocks/chaincodestub.go -fake-name ChaincodeStub . chaincodeStub
type chaincodeStub interface {
	shim.ChaincodeStubInterface
}

//go:generate counterfeiter -o mocks/statequeryiterator.go -fake-name StateQueryIterator . stateQueryIterator
type stateQueryIterator interface {
	shim.StateQueryIteratorInterface
}

type MockWorldState struct {
	data map[string][]byte
}

func (s *MockWorldState) ResetState() {
	s.data = make(map[string][]byte)
}

func (s *MockWorldState) PutState(key string, data []byte) error {
	s.data[key] = data
	return nil
}

func (s *MockWorldState) GetState(key string) ([]byte, error) {
	if _data, ok := s.data[key]; ok {
		return _data, nil
	} else {
		return nil, nil
	}
}

func MakeWorldState() MockWorldState {
	return MockWorldState{
		data: make(map[string][]byte),
	}
}

func TestCreateToken(t *testing.T) {
	token := Token{
		Id: "abc",
	}
	require.Equal(t, token.Id, "abc")
	chaincodeStub := &mocks.ChaincodeStub{}
	transactionContext := &mocks.TransactionContext{}
	transactionContext.GetStubReturns(chaincodeStub)

	mockWorldState := MakeWorldState()

	chaincodeStub.PutStateStub = mockWorldState.PutState
	chaincodeStub.GetStateStub = mockWorldState.GetState

	tokenContract := TokenContract{}
	err := tokenContract.CreateToken(
		transactionContext,
		"seed",
		AlwaysAcceptUrl,
		AlwaysSendUrl,
		"publickey",
	)
	require.NoError(t, err, "Failed to create token")

	tokenIdBytes := sha512.Sum512([]byte("seed"))
	tokenId := hex.EncodeToString(tokenIdBytes[:])
	tokenJson, err := mockWorldState.GetState(tokenId)
	require.NoError(t, err, "Failed to read token")
	require.NotNil(t, tokenJson, "Token not found")

	var retrievedToken Token
	err = json.Unmarshal(tokenJson, &retrievedToken)
	require.NoError(t, err)
	require.Equal(t, tokenId, retrievedToken.Id)
	require.Equal(t, make([]string, 0), retrievedToken.ConsumedTokenIds)
	require.Equal(t, "", retrievedToken.ConsumingTokenId)
	require.Equal(t, AlwaysAcceptUrl, retrievedToken.RequestToAcceptUrl)
	require.Equal(t, AlwaysSendUrl, retrievedToken.RequestToSendUrl)
	require.Equal(t, "publickey", retrievedToken.OwnerPublicKey)
}

func TestReadToken(t *testing.T) {
	chaincodeStub := &mocks.ChaincodeStub{}
	transactionContext := &mocks.TransactionContext{}
	transactionContext.GetStubReturns(chaincodeStub)

	mockWorldState := MakeWorldState()

	chaincodeStub.PutStateStub = mockWorldState.PutState
	chaincodeStub.GetStateStub = mockWorldState.GetState

	tokenContract := TokenContract{}
	err := tokenContract.CreateToken(
		transactionContext,
		"seed",
		AlwaysAcceptUrl,
		AlwaysSendUrl,
		"publickey",
	)
	require.NoError(t, err, "Failed to create token")

	tokenIdBytes := sha512.Sum512([]byte("seed"))
	tokenId := hex.EncodeToString(tokenIdBytes[:])
	retrievedToken, err := tokenContract.GetToken(
		transactionContext,
		tokenId,
	)

	require.NoError(t, err, "Failed to get token")
	require.NotNil(t, retrievedToken, "Could not get token")
	require.Equal(t, retrievedToken.Id, tokenId)
	require.Equal(t, retrievedToken.ConsumedTokenIds, make([]string, 0))
	require.Equal(t, retrievedToken.ConsumingTokenId, "")
	require.Equal(t, "publickey", retrievedToken.OwnerPublicKey)
}

func TestCannotCreateDuplicateToken(t *testing.T) {
	chaincodeStub := &mocks.ChaincodeStub{}
	transactionContext := &mocks.TransactionContext{}
	transactionContext.GetStubReturns(chaincodeStub)

	mockWorldState := MakeWorldState()

	chaincodeStub.PutStateStub = mockWorldState.PutState
	chaincodeStub.GetStateStub = mockWorldState.GetState

	tokenContract := TokenContract{}
	err := tokenContract.CreateToken(
		transactionContext,
		"seed",
		AlwaysAcceptUrl,
		AlwaysSendUrl,
		"publickey",
	)
	require.NoError(t, err, "Failed to create 1st token")

	err = tokenContract.CreateToken(
		transactionContext,
		"seed",
		AlwaysAcceptUrl,
		"abc",
		"publickey",
	)
	require.IsType(t, &AlreadyExistsError{}, err, "Duplicate token must not be created")

	tokenIdBytes := sha512.Sum512([]byte("seed"))
	tokenId := hex.EncodeToString(tokenIdBytes[:])
	tokenJson, err := chaincodeStub.GetState(tokenId)

	var retrievedToken Token
	err = json.Unmarshal(tokenJson, &retrievedToken)
	require.NoError(t, err)
	require.Equal(t, tokenId, retrievedToken.Id)
	require.Equal(t, make([]string, 0), retrievedToken.ConsumedTokenIds)
	require.Equal(t, "", retrievedToken.ConsumingTokenId)
	require.Equal(t, AlwaysAcceptUrl, retrievedToken.RequestToAcceptUrl)
	require.Equal(t, AlwaysSendUrl, retrievedToken.RequestToSendUrl)
	require.Equal(t, "publickey", retrievedToken.OwnerPublicKey)
}

func TestConsumeToken(t *testing.T) {
	chaincodeStub := &mocks.ChaincodeStub{}
	transactionContext := &mocks.TransactionContext{}
	transactionContext.GetStubReturns(chaincodeStub)

	mockWorldState := MakeWorldState()

	chaincodeStub.PutStateStub = mockWorldState.PutState
	chaincodeStub.GetStateStub = mockWorldState.GetState

	tokenContract := TokenContract{}
	err := tokenContract.CreateToken(
		transactionContext,
		"seed",
		AlwaysAcceptUrl,
		AlwaysSendUrl,
		"publickey",
	)
	require.NoError(t, err, "Failed to create 1st token")

	err = tokenContract.CreateToken(
		transactionContext,
		"seed-2",
		AlwaysAcceptUrl,
		AlwaysSendUrl,
		"publickey-2",
	)
	require.NoError(t, err, "Failed to create 2nd token")

	tokenIdBytes_1 := sha512.Sum512([]byte("seed"))
	tokenId_1 := hex.EncodeToString(tokenIdBytes_1[:])

	tokenIdBytes_2 := sha512.Sum512([]byte("seed-2"))
	tokenId_2 := hex.EncodeToString(tokenIdBytes_2[:])

	err = tokenContract.ConsumeToken(
		transactionContext,
		tokenId_1,
		tokenId_2,
	)
	require.NoError(t, err)

	retrievedToken_1, err := tokenContract.GetToken(
		transactionContext,
		tokenId_1,
	)

	retrievedToken_2, err := tokenContract.GetToken(
		transactionContext,
		tokenId_2,
	)

	require.Equal(t, tokenId_2, retrievedToken_1.ConsumingTokenId)
	require.Equal(t, []string{tokenId_1}, retrievedToken_2.ConsumedTokenIds)
}

/*
func TestCreateAsset(t *testing.T) {
	chaincodeStub := &mocks.ChaincodeStub{}
	transactionContext := &mocks.TransactionContext{}
	transactionContext.GetStubReturns(chaincodeStub)

	assetTransfer := chaincode.SmartContract{}
	err := assetTransfer.CreateAsset(transactionContext, "", "", 0, "", 0)
	require.NoError(t, err)

	chaincodeStub.GetStateReturns([]byte{}, nil)
	err = assetTransfer.CreateAsset(transactionContext, "asset1", "", 0, "", 0)
	require.EqualError(t, err, "the asset asset1 already exists")

	chaincodeStub.GetStateReturns(nil, fmt.Errorf("unable to retrieve asset"))
	err = assetTransfer.CreateAsset(transactionContext, "asset1", "", 0, "", 0)
	require.EqualError(t, err, "failed to read from world state: unable to retrieve asset")
}

func TestReadAsset(t *testing.T) {
	chaincodeStub := &mocks.ChaincodeStub{}
	transactionContext := &mocks.TransactionContext{}
	transactionContext.GetStubReturns(chaincodeStub)

	expectedAsset := &chaincode.Asset{ID: "asset1"}
	bytes, err := json.Marshal(expectedAsset)
	require.NoError(t, err)

	chaincodeStub.GetStateReturns(bytes, nil)
	assetTransfer := chaincode.SmartContract{}
	asset, err := assetTransfer.ReadAsset(transactionContext, "")
	require.NoError(t, err)
	require.Equal(t, expectedAsset, asset)

	chaincodeStub.GetStateReturns(nil, fmt.Errorf("unable to retrieve asset"))
	_, err = assetTransfer.ReadAsset(transactionContext, "")
	require.EqualError(t, err, "failed to read from world state: unable to retrieve asset")

	chaincodeStub.GetStateReturns(nil, nil)
	asset, err = assetTransfer.ReadAsset(transactionContext, "asset1")
	require.EqualError(t, err, "the asset asset1 does not exist")
	require.Nil(t, asset)
}

func TestUpdateAsset(t *testing.T) {
	chaincodeStub := &mocks.ChaincodeStub{}
	transactionContext := &mocks.TransactionContext{}
	transactionContext.GetStubReturns(chaincodeStub)

	expectedAsset := &chaincode.Asset{ID: "asset1"}
	bytes, err := json.Marshal(expectedAsset)
	require.NoError(t, err)

	chaincodeStub.GetStateReturns(bytes, nil)
	assetTransfer := chaincode.SmartContract{}
	err = assetTransfer.UpdateAsset(transactionContext, "", "", 0, "", 0)
	require.NoError(t, err)

	chaincodeStub.GetStateReturns(nil, nil)
	err = assetTransfer.UpdateAsset(transactionContext, "asset1", "", 0, "", 0)
	require.EqualError(t, err, "the asset asset1 does not exist")

	chaincodeStub.GetStateReturns(nil, fmt.Errorf("unable to retrieve asset"))
	err = assetTransfer.UpdateAsset(transactionContext, "asset1", "", 0, "", 0)
	require.EqualError(t, err, "failed to read from world state: unable to retrieve asset")
}

func TestDeleteAsset(t *testing.T) {
	chaincodeStub := &mocks.ChaincodeStub{}
	transactionContext := &mocks.TransactionContext{}
	transactionContext.GetStubReturns(chaincodeStub)

	asset := &chaincode.Asset{ID: "asset1"}
	bytes, err := json.Marshal(asset)
	require.NoError(t, err)

	chaincodeStub.GetStateReturns(bytes, nil)
	chaincodeStub.DelStateReturns(nil)
	assetTransfer := chaincode.SmartContract{}
	err = assetTransfer.DeleteAsset(transactionContext, "")
	require.NoError(t, err)

	chaincodeStub.GetStateReturns(nil, nil)
	err = assetTransfer.DeleteAsset(transactionContext, "asset1")
	require.EqualError(t, err, "the asset asset1 does not exist")

	chaincodeStub.GetStateReturns(nil, fmt.Errorf("unable to retrieve asset"))
	err = assetTransfer.DeleteAsset(transactionContext, "")
	require.EqualError(t, err, "failed to read from world state: unable to retrieve asset")
}

func TestTransferAsset(t *testing.T) {
	chaincodeStub := &mocks.ChaincodeStub{}
	transactionContext := &mocks.TransactionContext{}
	transactionContext.GetStubReturns(chaincodeStub)

	asset := &chaincode.Asset{ID: "asset1"}
	bytes, err := json.Marshal(asset)
	require.NoError(t, err)

	chaincodeStub.GetStateReturns(bytes, nil)
	assetTransfer := chaincode.SmartContract{}
	_, err = assetTransfer.TransferAsset(transactionContext, "", "")
	require.NoError(t, err)

	chaincodeStub.GetStateReturns(nil, fmt.Errorf("unable to retrieve asset"))
	_, err = assetTransfer.TransferAsset(transactionContext, "", "")
	require.EqualError(t, err, "failed to read from world state: unable to retrieve asset")
}

func TestGetAllAssets(t *testing.T) {
	asset := &chaincode.Asset{ID: "asset1"}
	bytes, err := json.Marshal(asset)
	require.NoError(t, err)

	iterator := &mocks.StateQueryIterator{}
	iterator.HasNextReturnsOnCall(0, true)
	iterator.HasNextReturnsOnCall(1, false)
	iterator.NextReturns(&queryresult.KV{Value: bytes}, nil)

	chaincodeStub := &mocks.ChaincodeStub{}
	transactionContext := &mocks.TransactionContext{}
	transactionContext.GetStubReturns(chaincodeStub)

	chaincodeStub.GetStateByRangeReturns(iterator, nil)
	assetTransfer := &chaincode.SmartContract{}
	assets, err := assetTransfer.GetAllAssets(transactionContext)
	require.NoError(t, err)
	require.Equal(t, []*chaincode.Asset{asset}, assets)

	iterator.HasNextReturns(true)
	iterator.NextReturns(nil, fmt.Errorf("failed retrieving next item"))
	assets, err = assetTransfer.GetAllAssets(transactionContext)
	require.EqualError(t, err, "failed retrieving next item")
	require.Nil(t, assets)

	chaincodeStub.GetStateByRangeReturns(nil, fmt.Errorf("failed retrieving all assets"))
	assets, err = assetTransfer.GetAllAssets(transactionContext)
	require.EqualError(t, err, "failed retrieving all assets")
	require.Nil(t, assets)
}
*/
