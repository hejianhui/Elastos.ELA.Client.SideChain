package wallet

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"math/big"
	"os"
	"math"
	"strconv"
	"strings"

	"github.com/elastos/Elastos.ELA.Client.SideChain/config"
	"github.com/elastos/Elastos.ELA.Client.SideChain/log"
	"github.com/elastos/Elastos.ELA.Client.SideChain/rpc"
	walt "github.com/elastos/Elastos.ELA.Client.SideChain/wallet"
	. "github.com/elastos/Elastos.ELA.SideChain/core"
	. "github.com/elastos/Elastos.ELA.Utility/common"
	"github.com/elastos/Elastos.ELA.Utility/crypto"
	"github.com/urfave/cli"
)

func createTransaction(c *cli.Context, wallet walt.Wallet) error {

	feeStr := c.String("fee")
	if feeStr == "" {
		return errors.New("use --fee to specify transfer fee")
	}

	fee, err := StringToFixed64(feeStr)
	if err != nil {
		return errors.New("invalid transaction fee")
	}

	from := c.String("from")
	if from == "" {
		from, err = SelectAccount(wallet)
		if err != nil {
			return err
		}
	}

	multiOutput := c.String("file")
	if multiOutput != "" {
		return createMultiOutputTransaction(c, wallet, multiOutput, from, fee)
	}

	amountStr := c.String("amount")
	if amountStr == "" {
		return errors.New("use --amount to specify transfer amount")
	}
	var amountEla *Fixed64 //fixed64 or big.Int?
	var amountBigFloat *big.Float
	var amountInt int64
	asset := c.String("asset")
	if asset == "" || asset == walt.SystemAssetId.String() {
		amountEla, err = StringToFixed64(amountStr)
	} else {
		var success bool
		fmt.Println("amountStr:", amountStr)
		amountBigFloat, success = new(big.Float).SetString(amountStr)
		if !success {
			err = errors.New("parse string to big.Int failed.")
		}
	}
	if err != nil {
		return errors.New("invalid transaction amount: " + err.Error())
	}

	var txn *Transaction
	var to string
	standard := c.String("to")
	deposit := c.String("deposit")
	withdraw := c.String("withdraw")
	register := c.String("register")
	if register != "" {
		amountInt, _ = strconv.ParseInt(amountStr, 10, 64)
		amountIntFixed64 := Fixed64(amountInt)
		assetname := c.String("assetname")
		if amountStr == "" {
			return errors.New("use --assetname to specify asset name")
		}
		description := c.String("description")
		precision := c.Uint("precision")
		txn, err = wallet.CreateRegisterTransaction(from, register, &Asset{
			Name:        assetname,
			Description: description,
			Precision:   byte(precision),
			AssetType:   0x00,
		}, &amountIntFixed64, fee)
		if err != nil {
			return errors.New("create transaction failed: " + err.Error())
		}
	} else if deposit != "" {
		to = config.Params().DepositAddress
		txn, err = wallet.CreateCrossChainTransaction(from, to, deposit, amountEla, fee)
		if err != nil {
			return errors.New("create transaction failed: " + err.Error())
		}
	} else if withdraw != "" {
		to = walt.DESTROY_ADDRESS
		txn, err = wallet.CreateCrossChainTransaction(from, to, withdraw, amountEla, fee)
		if err != nil {
			return errors.New("create transaction failed: " + err.Error())
		}
	} else if standard != "" {
		to = standard
		lockStr := c.String("lock")
		if lockStr == "" {
			lockStr = "0"
		}
		asset := c.String("asset")
		lock, err := strconv.ParseUint(lockStr, 10, 32)
		if asset == "" {
			asset = "a3d0eaa466df74983b5d7c543de6904f4c9418ead5ffd6d25814234a96db37b0"
		}
		if err != nil {
			return errors.New("parse utxo lock failed." + err.Error())
		}
		assetIDBytes, err := HexStringToBytes(asset)
		if err != nil {
			return errors.New("invalid asset id")
		}
		assetID, err := Uint256FromBytes(BytesReverse(assetIDBytes))
		if err != nil {
			return errors.New("invalid asset id")
		}
		if *assetID == EmptyHash || *assetID == walt.SystemAssetId {
			// create ela tx
			txn, err = wallet.CreateLockedTransaction(from, to, amountEla, fee, uint32(lock))
			if err != nil {
				return errors.New("create ELA transaction failed: " + err.Error())
			}
		} else {
			amountBigInt, _ := new(big.Float).Mul(amountBigFloat, big.NewFloat(math.Pow10(MaxPrecision))).Int(nil)
			txn, err = wallet.CreateLockedTokenTransaction(from, to, amountBigInt, fee, assetID, uint32(lock))

			if err != nil {
				return errors.New("create token transaction failed: " + err.Error())
			}
		}
	} else {
		return errors.New("use --to or --deposit or --withdraw to specify receiver address")
	}

	output(0, 0, txn)

	return nil
}

func createMultiOutputTransaction(c *cli.Context, wallet walt.Wallet, path, from string, fee *Fixed64) error {
	if _, err := os.Stat(path); err != nil {
		return errors.New("invalid multi output file path")
	}
	file, err := os.OpenFile(path, os.O_RDONLY, 0666)
	if err != nil {
		return errors.New("open multi output file failed")
	}

	scanner := bufio.NewScanner(file)
	var multiOutput []*walt.Transfer
	for scanner.Scan() {
		columns := strings.Split(scanner.Text(), ",")
		if len(columns) < 2 {
			return errors.New(fmt.Sprint("invalid multi output line:", columns))
		}
		amountStr := strings.TrimSpace(columns[1])
		amount, err := StringToFixed64(amountStr)
		if err != nil {
			return errors.New("invalid multi output transaction amount: " + amountStr)
		}
		address := strings.TrimSpace(columns[0])
		multiOutput = append(multiOutput, &walt.Transfer{address, amount, new(big.Int)})
		log.Trace("Multi output address:", address, ", amount:", amountStr)
	}

	lockStr := c.String("lock")
	var txn *Transaction
	if lockStr == "" {
		txn, err = wallet.CreateMultiOutputTransaction(from, fee, multiOutput...)
		if err != nil {
			return errors.New("create multi output transaction failed: " + err.Error())
		}
	} else {
		lock, err := strconv.ParseUint(lockStr, 10, 32)
		if err != nil {
			return errors.New("invalid lock height")
		}
		txn, err = wallet.CreateLockedMultiOutputTransaction(from, fee, uint32(lock), multiOutput...)
		if err != nil {
			return errors.New("create multi output transaction failed: " + err.Error())
		}
	}

	output(0, 0, txn)

	return nil
}

func signTransaction(name string, password []byte, context *cli.Context, wallet walt.Wallet) error {
	defer ClearBytes(password)

	content, err := getTransactionContent(context)
	if err != nil {
		return err
	}
	rawData, err := HexStringToBytes(content)
	if err != nil {
		return errors.New("decode transaction content failed")
	}

	var txn Transaction
	err = txn.Deserialize(bytes.NewReader(rawData))
	if err != nil {
		return errors.New("deserialize transaction failed")
	}

	program := txn.Programs[0]

	haveSign, needSign, err := crypto.GetSignStatus(program.Code, program.Parameter)
	if haveSign == needSign {
		return errors.New("transaction was fully signed, no need more sign")
	}

	password, err = GetPassword(password, false)
	if err != nil {
		return err
	}

	_, err = wallet.Sign(name, password, &txn)
	if err != nil {
		return err
	}

	haveSign, needSign, _ = crypto.GetSignStatus(program.Code, program.Parameter)
	fmt.Println("[", haveSign, "/", needSign, "] Transaction successfully signed")

	output(haveSign, needSign, &txn)

	return nil
}

func sendTransaction(context *cli.Context) error {
	content, err := getTransactionContent(context)
	if err != nil {
		return err
	}

	result, err := rpc.CallAndUnmarshal("sendrawtransaction", rpc.Param("data", content))
	if err != nil {
		return err
	}
	fmt.Println(result.(string))
	return nil
}

func getTransactionContent(context *cli.Context) (string, error) {

	// If parameter with file path is not empty, read content from file
	if filePath := strings.TrimSpace(context.String("file")); filePath != "" {

		if _, err := os.Stat(filePath); err != nil {
			return "", errors.New("invalid transaction file path")
		}
		file, err := os.OpenFile(filePath, os.O_RDONLY, 0666)
		if err != nil {
			return "", errors.New("open transaction file failed")
		}
		rawData, err := ioutil.ReadAll(file)
		if err != nil {
			return "", errors.New("read transaction file failed")
		}

		content := strings.TrimSpace(string(rawData))
		// File content can not by empty
		if content == "" {
			return "", errors.New("transaction file is empty")
		}
		return content, nil
	}

	content := strings.TrimSpace(context.String("hex"))
	// Hex string content can not be empty
	if content == "" {
		return "", errors.New("transaction hex string is empty")
	}

	return content, nil
}

func output(haveSign, needSign int, txn *Transaction) error {
	// Serialise transaction content
	buf := new(bytes.Buffer)
	txn.Serialize(buf)
	content := BytesToHexString(buf.Bytes())

	// Print transaction hex string content to console
	fmt.Println(content)

	// Output to file
	fileName := "to_be_signed" // Create transaction file name

	if haveSign == 0 {
		//	Transaction created do nothing
	} else if needSign > haveSign {
		fileName = fmt.Sprint(fileName, "_", haveSign, "_of_", needSign)
	} else if needSign == haveSign {
		fileName = "ready_to_send"
	}
	fileName = fileName + ".txn"

	file, err := os.OpenFile(fileName, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
		return err
	}

	_, err = file.Write([]byte(content))
	if err != nil {
		return err
	}

	// Print output file to console
	fmt.Println("File: ", fileName)

	return nil
}
