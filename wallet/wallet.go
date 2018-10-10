package wallet

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"math/big"

	"github.com/elastos/Elastos.ELA.Client.SideChain/log"

	. "github.com/elastos/Elastos.ELA.Utility/common"
	"github.com/elastos/Elastos.ELA.Utility/crypto"
	"github.com/elastos/Elastos.ELA.SideChain/types"
)

const (
	DESTROY_ADDRESS = "0000000000000000000000000000000000"
)

var SystemAssetId = getSystemAssetId()

type Transfer struct {
	Address string
	Amount  *Fixed64
}

type TransferToken struct {
	Address string
	Amount  *big.Int
}

type CrossChainOutput struct {
	Address           string
	Amount            *Fixed64
	CrossChainAddress string
}

var wallet Wallet // Single instance of wallet

type Wallet interface {
	DataStore

	Open(name string, password []byte) error
	ChangePassword(oldPassword, newPassword []byte) error

	AddStandardAccount(publicKey *crypto.PublicKey) (*Uint168, error)
	AddMultiSignAccount(M uint, publicKey ...*crypto.PublicKey) (*Uint168, error)

	CreateTransaction(fromAddress, toAddress string, amount, fee *Fixed64) (*types.Transaction, error)
	CreateLockedTransaction(fromAddress, toAddress string, amount, fee *Fixed64, lockedUntil uint32) (*types.Transaction, error)
	CreateMultiOutputTransaction(fromAddress string, fee *Fixed64, output ...*Transfer) (*types.Transaction, error)
	CreateLockedMultiOutputTransaction(fromAddress string, fee *Fixed64, lockedUntil uint32, output ...*Transfer) (*types.Transaction, error)
	CreateCrossChainTransaction(fromAddress, toAddress, crossChainAddress string, amount, fee *Fixed64) (*types.Transaction, error)
	CreateTokenTransaction(fromAddress, toAddress string, amount *big.Int, fee *Fixed64, assetID *Uint256) (*types.Transaction, error)
	CreateLockedTokenTransaction(fromAddress, toAddress string, amount *big.Int, fee *Fixed64, assetID *Uint256, lockedUntil uint32) (*types.Transaction, error)
	CreateRegisterTransaction(fromAddress, regAddress string, asset *types.Asset, regAmount *big.Int, fee *Fixed64) (*types.Transaction, error)

	Sign(name string, password []byte, transaction *types.Transaction) (*types.Transaction, error)

	Reset() error
}

type WalletImpl struct {
	DataStore
	Keystore
}

func Create(name string, password []byte) (*WalletImpl, error) {
	keyStore, err := CreateKeystore(name, password)
	if err != nil {
		log.Error("Wallet create key store failed:", err)
		return nil, err
	}

	dataStore, err := OpenDataStore()
	if err != nil {
		log.Error("Wallet create data store failed:", err)
		return nil, err
	}

	dataStore.AddAddress(keyStore.GetProgramHash(), keyStore.GetRedeemScript(), TypeMaster)

	return &WalletImpl{
		DataStore: dataStore,
		Keystore:  keyStore,
	}, nil
}

func GetWallet() (*WalletImpl, error) {
	dataStore, err := OpenDataStore()
	if err != nil {
		return nil, err
	}

	return &WalletImpl{
		DataStore: dataStore,
	}, nil
}

func (wallet *WalletImpl) Open(name string, password []byte) error {
	keyStore, err := OpenKeystore(name, password)
	if err != nil {
		return err
	}
	wallet.Keystore = keyStore
	return nil
}

func (wallet *WalletImpl) AddStandardAccount(publicKey *crypto.PublicKey) (*Uint168, error) {
	redeemScript, err := crypto.CreateStandardRedeemScript(publicKey)
	if err != nil {
		return nil, errors.New("[Wallet], CreateStandardRedeemScript failed")
	}

	programHash, err := crypto.ToProgramHash(redeemScript)
	if err != nil {
		return nil, errors.New("[Wallet], CreateStandardAddress failed")
	}

	err = wallet.AddAddress(programHash, redeemScript, TypeStand)
	if err != nil {
		return nil, err
	}

	return programHash, nil
}

func (wallet *WalletImpl) AddMultiSignAccount(M uint, publicKeys ...*crypto.PublicKey) (*Uint168, error) {
	redeemScript, err := crypto.CreateMultiSignRedeemScript(M, publicKeys)
	if err != nil {
		return nil, errors.New("[Wallet], CreateStandardRedeemScript failed")
	}

	programHash, err := crypto.ToProgramHash(redeemScript)
	if err != nil {
		return nil, errors.New("[Wallet], CreateMultiSignAddress failed")
	}

	err = wallet.AddAddress(programHash, redeemScript, TypeMulti)
	if err != nil {
		return nil, err
	}

	return programHash, nil
}

func (wallet *WalletImpl) CreateTransaction(fromAddress, toAddress string, amount, fee *Fixed64) (*types.Transaction, error) {
	return wallet.CreateLockedTransaction(fromAddress, toAddress, amount, fee, uint32(0))
}

func (wallet *WalletImpl) CreateLockedTransaction(fromAddress, toAddress string, amount, fee *Fixed64, lockedUntil uint32) (*types.Transaction, error) {
	return wallet.CreateLockedMultiOutputTransaction(fromAddress, fee, lockedUntil, &Transfer{toAddress, amount})
}

func (wallet *WalletImpl) CreateMultiOutputTransaction(fromAddress string, fee *Fixed64, outputs ...*Transfer) (*types.Transaction, error) {
	return wallet.CreateLockedMultiOutputTransaction(fromAddress, fee, uint32(0), outputs...)
}

func (wallet *WalletImpl) CreateLockedMultiOutputTransaction(fromAddress string, fee *Fixed64, lockedUntil uint32, outputs ...*Transfer) (*types.Transaction, error) {
	return wallet.createTransaction(fromAddress, fee, lockedUntil, outputs...)
}

func (wallet *WalletImpl) CreateCrossChainTransaction(fromAddress, toAddress, crossChainAddress string, amount, fee *Fixed64) (*types.Transaction, error) {
	return wallet.createCrossChainTransaction(fromAddress, fee, uint32(0), &CrossChainOutput{toAddress, amount, crossChainAddress})
}

func (wallet *WalletImpl) CreateTokenTransaction(fromAddress, toAddress string, amount *big.Int, fee *Fixed64, assetID *Uint256) (*types.Transaction, error) {
	if assetID == &SystemAssetId {
		value := Fixed64(amount.Int64())
		return wallet.CreateLockedTransaction(fromAddress, toAddress, &value, fee, uint32(0))
	}
	return wallet.CreateLockedTokenTransaction(fromAddress, toAddress, amount, fee, assetID, uint32(0))
}

func (wallet *WalletImpl) CreateLockedTokenTransaction(fromAddress, toAddress string, amount *big.Int, fee *Fixed64, assetID *Uint256, lockedUntil uint32) (*types.Transaction, error) {
	if assetID == &SystemAssetId {
		value := Fixed64(amount.Int64())
		return wallet.createTransaction(fromAddress, fee, lockedUntil, &Transfer{toAddress, &value})
	}
	return wallet.createTokenTransaction(fromAddress, assetID, fee, lockedUntil, &TransferToken{toAddress, amount})
}

func (wallet *WalletImpl) CreateRegisterTransaction(fromAddress, regAddress string, asset *types.Asset, regAmount *big.Int, fee *Fixed64) (*types.Transaction, error) {
	return wallet.createRegisterTransaction(fromAddress, fee, uint32(0), asset, regAmount, regAddress)
}

func (wallet *WalletImpl) createTransaction(fromAddress string, fee *Fixed64, lockedUntil uint32, outputs ...*Transfer) (*types.Transaction, error) {
	// Check if output is valid
	if len(outputs) == 0 {
		return nil, errors.New("[Wallet], Invalid transaction target")
	}
	// Sync chain block data before create transaction
	wallet.SyncChainData()

	// Check if from address is valid
	spender, err := Uint168FromAddress(fromAddress)
	if err != nil {
		return nil, errors.New(fmt.Sprint("[Wallet], Invalid spender address: ", fromAddress, ", error: ", err))
	}
	// Create transaction outputs
	var totalOutputAmount = Fixed64(0) // The total amount will be spend
	var txOutputs []*types.Output            // The outputs in transaction
	totalOutputAmount += *fee          // Add transaction fee

	for _, output := range outputs {
		receiver, err := Uint168FromAddress(output.Address)
		if err != nil {
			return nil, errors.New(fmt.Sprint("[Wallet], Invalid receiver address: ", output.Address, ", error: ", err))
		}
		txOutput := &types.Output{
			AssetID:     SystemAssetId,
			ProgramHash: *receiver,
			Value:       *output.Amount,
			OutputLock:  lockedUntil,
		}
		totalOutputAmount += *output.Amount
		txOutputs = append(txOutputs, txOutput)
	}
	// Get spender's UTXOs
	UTXOs, err := wallet.GetAddressUTXOs(spender, &SystemAssetId)
	if err != nil {
		return nil, errors.New("[Wallet], Get spender's UTXOs failed")
	}
	availableUTXOs := wallet.removeLockedUTXOs(UTXOs) // Remove locked UTXOs
	availableUTXOs = SortUTXOs(availableUTXOs)        // Sort available UTXOs by value ASC

	// Create transaction inputs
	var txInputs []*types.Input // The inputs in transaction
	for _, utxo := range availableUTXOs {
		var amount Fixed64
		reader := bytes.NewReader(utxo.Amount)
		amount.Deserialize(reader)
		if amount == Fixed64(0) {
			continue
		}
		input := &types.Input{
			Previous: types.OutPoint{
				TxID:  utxo.Op.TxID,
				Index: utxo.Op.Index,
			},
			Sequence: utxo.LockTime,
		}
		txInputs = append(txInputs, input)
		if amount < totalOutputAmount {
			totalOutputAmount -= amount
		} else if amount == totalOutputAmount {
			totalOutputAmount = 0
			break
		} else if amount > totalOutputAmount {
			change := &types.Output{
				AssetID:     SystemAssetId,
				Value:       amount - totalOutputAmount,
				OutputLock:  uint32(0),
				ProgramHash: *spender,
			}
			txOutputs = append(txOutputs, change)
			totalOutputAmount = 0
			break
		}
	}
	if totalOutputAmount > 0 {
		return nil, errors.New("[Wallet], Available token is not enough")
	}

	account, err := wallet.GetAddressInfo(spender)
	if err != nil {
		return nil, errors.New("[Wallet], Get spenders account info failed")
	}
	payload := &types.PayloadTransferAsset{}
	return wallet.newTransaction(account.RedeemScript, txInputs, txOutputs, payload, types.TransferAsset), nil
}

func (wallet *WalletImpl) createRegisterTransaction(fromAddress string, fee *Fixed64, lockedUntil uint32, asset *types.Asset, regAmount *big.Int, regAddress string) (*types.Transaction, error) {
	// Sync chain block data before create transaction
	wallet.SyncChainData()

	// Check if from address is valid
	spender, err := Uint168FromAddress(fromAddress)
	if err != nil {
		return nil, errors.New(fmt.Sprint("[Wallet], Invalid spender address: ", fromAddress, ", error: ", err))
	}
	// Create transaction outputs
	var totalOutputAmount = Fixed64(0) // The total amount will be spend
	var txOutputs []*types.Output            // The outputs in transaction
	totalOutputAmount = *fee           // Add transaction fee

	var payload *types.PayloadRegisterAsset
	registerAddr, err := Uint168FromAddress(regAddress)
	if err != nil {
		return nil, errors.New(fmt.Sprint("[Wallet], Invalid register address: ", regAddress, ", error: ", err))
	}
	payload = &types.PayloadRegisterAsset{
		Asset:      *asset,
		Amount:    Fixed64(regAmount.Int64()),
		Controller: *registerAddr,
	}
	buf := new(bytes.Buffer)
	payload.Asset.Serialize(buf)
	assetHash := Sha256D(buf.Bytes())
	change := &types.Output{
		AssetID:     assetHash,
		TokenValue: *new(big.Int).Mul(regAmount, getTokenPrecisionBigInt()),
		OutputLock:  uint32(0),
		ProgramHash: *registerAddr,
	}
	txOutputs = append(txOutputs, change)

	// Get spender's UTXOs
	UTXOs, err := wallet.GetAddressUTXOs(spender, &SystemAssetId)
	if err != nil {
		return nil, errors.New("[Wallet], Get spender's UTXOs failed")
	}
	availableUTXOs := wallet.removeLockedUTXOs(UTXOs) // Remove locked UTXOs
	availableUTXOs = SortUTXOs(availableUTXOs)        // Sort available UTXOs by value ASC

	// Create transaction inputs
	var txInputs []*types.Input // The inputs in transaction
	for _, utxo := range availableUTXOs {
		var amount Fixed64
		reader := bytes.NewReader(utxo.Amount)
		amount.Deserialize(reader)
		if amount == Fixed64(0) {
			continue
		}
		input := &types.Input{
			Previous: types.OutPoint{
				TxID:  utxo.Op.TxID,
				Index: utxo.Op.Index,
			},
			Sequence: utxo.LockTime,
		}
		txInputs = append(txInputs, input)
		if amount < totalOutputAmount {
			totalOutputAmount -= amount
		} else if amount == totalOutputAmount {
			totalOutputAmount = 0
			break
		} else if amount > totalOutputAmount {
			change := &types.Output{
				AssetID:     SystemAssetId,
				Value:       amount - totalOutputAmount,
				OutputLock:  uint32(0),
				ProgramHash: *spender,
			}
			txOutputs = append(txOutputs, change)
			totalOutputAmount = 0
			break
		}
	}
	if totalOutputAmount > 0 {
		return nil, errors.New("[Wallet], Available token is not enough")
	}

	account, err := wallet.GetAddressInfo(spender)
	if err != nil {
		return nil, errors.New("[Wallet], Get spenders account info failed")
	}

	return wallet.newTransaction(account.RedeemScript, txInputs, txOutputs, payload, types.RegisterAsset), nil
}

func (wallet *WalletImpl) createCrossChainTransaction(fromAddress string, fee *Fixed64, lockedUntil uint32, outputs ...*CrossChainOutput) (*types.Transaction, error) {
	// Check if output is valid
	if len(outputs) == 0 {
		return nil, errors.New("[Wallet], Invalid transaction target")
	}
	// Sync chain block data before create transaction
	wallet.SyncChainData()

	// Check if from address is valid
	spender, err := Uint168FromAddress(fromAddress)
	if err != nil {
		return nil, errors.New(fmt.Sprint("[Wallet], Invalid spender address: ", fromAddress, ", error: ", err))
	}
	// Create transaction outputs
	var totalOutputAmount = Fixed64(0) // The total amount will be spend
	var txOutputs []*types.Output            // The outputs in transaction
	totalOutputAmount += *fee          // Add transaction fee
	perAccountFee := *fee / Fixed64(len(outputs))

	txPayload := &types.PayloadTransferCrossChainAsset{}
	for index, output := range outputs {
		var receiver *Uint168
		if output.Address == DESTROY_ADDRESS {
			receiver = &Uint168{}
		} else {
			receiver, err = Uint168FromAddress(output.Address)
			if err != nil {
				return nil, errors.New(fmt.Sprint("[Wallet], Invalid receiver address: ", output.Address, ", error: ", err))
			}
		}
		txOutput := &types.Output{
			AssetID:     SystemAssetId,
			ProgramHash: *receiver,
			Value:       *output.Amount,
			OutputLock:  lockedUntil,
		}
		totalOutputAmount += *output.Amount
		txOutputs = append(txOutputs, txOutput)

		txPayload.CrossChainAddresses = append(txPayload.CrossChainAddresses, output.CrossChainAddress)
		txPayload.OutputIndexes = append(txPayload.OutputIndexes, uint64(index))
		txPayload.CrossChainAmounts = append(txPayload.CrossChainAmounts, *output.Amount-perAccountFee)
	}
	// Get spender's UTXOs
	UTXOs, err := wallet.GetAddressUTXOs(spender, &SystemAssetId)
	if err != nil {
		return nil, errors.New("[Wallet], Get spender's UTXOs failed")
	}
	availableUTXOs := wallet.removeLockedUTXOs(UTXOs) // Remove locked UTXOs
	availableUTXOs = SortUTXOs(availableUTXOs)        // Sort available UTXOs by value ASC

	// Create transaction inputs
	var txInputs []*types.Input // The inputs in transaction
	for _, utxo := range availableUTXOs {
		var amount Fixed64
		reader := bytes.NewReader(utxo.Amount)
		amount.Deserialize(reader)
		if amount == Fixed64(0) {
			continue
		}
		input := &types.Input{
			Previous: types.OutPoint{
				TxID:  utxo.Op.TxID,
				Index: utxo.Op.Index,
			},
			Sequence: utxo.LockTime,
		}
		txInputs = append(txInputs, input)
		if amount < totalOutputAmount {
			totalOutputAmount -= amount
		} else if amount == totalOutputAmount {
			totalOutputAmount = 0
			break
		} else if amount > totalOutputAmount {
			change := &types.Output{
				AssetID:     SystemAssetId,
				Value:       amount - totalOutputAmount,
				OutputLock:  uint32(0),
				ProgramHash: *spender,
			}
			txOutputs = append(txOutputs, change)
			totalOutputAmount = 0
			break
		}
	}
	if totalOutputAmount > 0 {
		return nil, errors.New("[Wallet], Available token is not enough")
	}

	account, err := wallet.GetAddressInfo(spender)
	if err != nil {
		return nil, errors.New("[Wallet], Get spenders account info failed")
	}

	txn := wallet.newTransaction(account.RedeemScript, txInputs, txOutputs, txPayload, types.TransferCrossChainAsset)
	return txn, nil
}

func (wallet *WalletImpl) createTokenTransaction(fromAddress string, assetID *Uint256, fee *Fixed64, lockedUntil uint32, outputs ...*TransferToken) (*types.Transaction, error) {
	// Check if output is valid
	if len(outputs) == 0 {
		return nil, errors.New("[Wallet], Invalid transaction target")
	}
	// Sync chain block data before create transaction
	wallet.SyncChainData()

	// Check if from address is valid
	spender, err := Uint168FromAddress(fromAddress)
	if err != nil {
		return nil, errors.New(fmt.Sprint("[Wallet], Invalid token spender address: ", fromAddress, ", error: ", err))
	}

	// Create transaction outputs for token
	var totalOutputAmount = big.NewInt(0) // The total amount will be spend
	var txOutputs []*types.Output            // The outputs in transaction

	for _, output := range outputs {
		receiver, err := Uint168FromAddress(output.Address)
		if err != nil {
			return nil, errors.New(fmt.Sprint("[Wallet], Invalid receiver address: ", output.Address, ", error: ", err))
		}
		txOutput := &types.Output{
			AssetID:     *assetID,
			ProgramHash: *receiver,
			TokenValue:  *output.Amount,
			OutputLock:  lockedUntil,
		}
		totalOutputAmount.Add(totalOutputAmount, output.Amount)
		txOutputs = append(txOutputs, txOutput)
	}
	// Get token spender's UTXOs
	tokenUTXOs, err := wallet.GetAddressUTXOs(spender, assetID)
	if err != nil {
		return nil, errors.New("[Wallet], Get spender's UTXOs failed")
	}
	availableTokenUTXOs := wallet.removeLockedUTXOs(tokenUTXOs) // Remove locked UTXOs
	availableTokenUTXOs = SortUTXOs(availableTokenUTXOs)        // Sort available UTXOs by value ASC

	// Create transaction inputs for token
	var txInputs []*types.Input // The inputs in transaction
	for _, utxo := range availableTokenUTXOs {
		var amount big.Int
		amount.SetBytes(utxo.Amount)
		if amount.Sign() != 1 {
			continue
		}
		input := &types.Input{
			Previous: types.OutPoint{
				TxID:  utxo.Op.TxID,
				Index: utxo.Op.Index,
			},
			Sequence: utxo.LockTime,
		}
		txInputs = append(txInputs, input)
		if amount.Cmp(totalOutputAmount) < 0 {
			totalOutputAmount.Sub(totalOutputAmount, &amount)
		} else if amount.Cmp(totalOutputAmount) == 0 {
			totalOutputAmount = big.NewInt(0)
			break
		} else if amount.Cmp(totalOutputAmount) > 0 {
			change := &types.Output{
				AssetID:     *assetID,
				TokenValue:  *new(big.Int).Sub(&amount, totalOutputAmount),
				OutputLock:  uint32(0),
				ProgramHash: *spender,
			}
			txOutputs = append(txOutputs, change)
			totalOutputAmount = big.NewInt(0)
			break
		}
	}
	if totalOutputAmount.Sign() > 0 {
		return nil, errors.New("[Wallet], Available token is not enough")
	}

	// Get ela spender's UTXOs
	elaUTXOs, err := wallet.GetAddressUTXOs(spender, &SystemAssetId)
	if err != nil {
		return nil, errors.New("[Wallet], Get spender's UTXOs failed")
	}
	availableElaUTXOs := wallet.removeLockedUTXOs(elaUTXOs) // Remove locked UTXOs
	availableElaUTXOs = SortUTXOs(availableElaUTXOs)        // Sort available UTXOs by value ASC

	// Create transaction inputs for ela fee
	totalFee := *fee
	for _, utxo := range availableElaUTXOs {
		var amount Fixed64
		reader := bytes.NewReader(utxo.Amount)
		amount.Deserialize(reader)
		if amount == Fixed64(0) {
			continue
		}
		input := &types.Input{
			Previous: types.OutPoint{
				TxID:  utxo.Op.TxID,
				Index: utxo.Op.Index,
			},
			Sequence: utxo.LockTime,
		}
		txInputs = append(txInputs, input)
		if amount < *fee {
			totalFee -= amount
		} else if amount == totalFee {
			totalFee = 0
			break
		} else if amount > totalFee {
			change := &types.Output{
				AssetID:     SystemAssetId,
				Value:       amount - totalFee,
				OutputLock:  uint32(0),
				ProgramHash: *spender,
			}
			txOutputs = append(txOutputs, change)
			totalFee = 0
			break
		}
	}
	if totalFee > 0 {
		return nil, errors.New("[Wallet], Available ela is not enough")
	}

	account, err := wallet.GetAddressInfo(spender)
	if err != nil {
		return nil, errors.New("[Wallet], Get token spenders account info failed")
	}
	payload := &types.PayloadTransferAsset{}

	return wallet.newTransaction(account.RedeemScript, txInputs, txOutputs, payload, types.TransferAsset), nil
}

func (wallet *WalletImpl) Sign(name string, password []byte, txn *types.Transaction) (*types.Transaction, error) {
	// Verify password
	err := wallet.Open(name, password)
	if err != nil {
		return nil, err
	}
	// Get sign type
	signType, err := crypto.GetScriptType(txn.Programs[0].Code)
	if err != nil {
		return nil, err
	}
	// Look up transaction type
	if signType == STANDARD {

		// Sign single transaction
		txn, err = wallet.signStandardTransaction(txn)
		if err != nil {
			return nil, err
		}

	} else if signType == MULTISIG {

		// Sign multi sign transaction
		txn, err = wallet.signMultiSignTransaction(txn)
		if err != nil {
			return nil, err
		}
	}

	return txn, nil
}

func (wallet *WalletImpl) signStandardTransaction(txn *types.Transaction) (*types.Transaction, error) {
	code := txn.Programs[0].Code
	// Get signer
	programHash, err := crypto.GetSigner(code)
	// Check if current user is a valid signer
	if *programHash != *wallet.Keystore.GetProgramHash() {
		return nil, errors.New("[Wallet], Invalid signer")
	}
	// Sign transaction
	signedTx, err := wallet.Keystore.Sign(txn)
	if err != nil {
		return nil, err
	}
	// Add verify program for transaction
	buf := new(bytes.Buffer)
	buf.WriteByte(byte(len(signedTx)))
	buf.Write(signedTx)
	// Add signature
	txn.Programs[0].Parameter = buf.Bytes()

	return txn, nil
}

func (wallet *WalletImpl) signMultiSignTransaction(txn *types.Transaction) (*types.Transaction, error) {
	code := txn.Programs[0].Code
	param := txn.Programs[0].Parameter
	// Check if current user is a valid signer
	var signerIndex = -1
	programHashes, err := crypto.GetSigners(code)
	if err != nil {
		return nil, err
	}
	userProgramHash := wallet.Keystore.GetProgramHash()
	for i, programHash := range programHashes {
		if *userProgramHash == *programHash {
			signerIndex = i
			break
		}
	}
	if signerIndex == -1 {
		return nil, errors.New("[Wallet], Invalid multi sign signer")
	}
	// Sign transaction
	signature, err := wallet.Keystore.Sign(txn)
	if err != nil {
		return nil, err
	}
	// Append signature
	buf := new(bytes.Buffer)
	txn.SerializeUnsigned(buf)
	txn.Programs[0].Parameter, err = crypto.AppendSignature(signerIndex, signature, buf.Bytes(), code, param)
	if err != nil {
		return nil, err
	}

	return txn, nil
}

func (wallet *WalletImpl) Reset() error {
	return wallet.ResetDataStore()
}

func getSystemAssetId() Uint256 {
	systemToken := &types.Transaction{
		TxType:         types.RegisterAsset,
		PayloadVersion: 0,
		Payload: &types.PayloadRegisterAsset{
			Asset: types.Asset{
				Name:      "ELA",
				Precision: 0x08,
				AssetType: 0x00,
			},
			Amount:     0 * 100000000,
			Controller: Uint168{},
		},
		Attributes: []*types.Attribute{},
		Inputs:     []*types.Input{},
		Outputs:    []*types.Output{},
		Programs:   []*types.Program{},
	}
	return systemToken.Hash()
}

func (wallet *WalletImpl) removeLockedUTXOs(utxos []*UTXO) []*UTXO {
	var availableUTXOs []*UTXO
	var currentHeight = wallet.CurrentHeight(QueryHeightCode)
	for _, utxo := range utxos {
		if utxo.LockTime > 0 {
			if utxo.LockTime >= currentHeight {
				continue
			}
			utxo.LockTime = math.MaxUint32 - 1
		}
		availableUTXOs = append(availableUTXOs, utxo)
	}
	return availableUTXOs
}

func (wallet *WalletImpl) newTransaction(redeemScript []byte, inputs []*types.Input, outputs []*types.Output, txPayload types.Payload, txType types.TransactionType) *types.Transaction {
	// Create attributes
	txAttr := types.NewAttribute(types.Nonce, []byte(strconv.FormatInt(rand.Int63(), 10)))
	attributes := make([]*types.Attribute, 0)
	attributes = append(attributes, &txAttr)
	// Create program
	var program = &types.Program{redeemScript, nil}
	// Create transaction
	return &types.Transaction{
		TxType:     txType,
		Payload:    txPayload,
		Attributes: attributes,
		Inputs:     inputs,
		Outputs:    outputs,
		Programs:   []*types.Program{program},
		LockTime:   0,
	}
}

func getTokenPrecisionBigInt() *big.Int {
	value := big.Int{}
	value.SetString("1000000000000000000", 10)
	return &value
}