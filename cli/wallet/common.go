package wallet

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"bytes"
	"math/big"

	walt "github.com/elastos/Elastos.ELA.Client.SideChain/wallet"

	"github.com/howeyc/gopass"
	"github.com/cheggaaa/pb"
	. "github.com/elastos/Elastos.ELA.Utility/common"
)

func GetPassword(password []byte, confirmed bool) ([]byte, error) {
	if len(password) > 0 {
		return []byte(password), nil
	}

	fmt.Print("INPUT PASSWORD:")

	password, err := gopass.GetPasswd()
	if err != nil {
		return nil, err
	}

	if !confirmed {
		return password, nil
	} else {

		fmt.Print("CONFIRM PASSWORD:")

		confirm, err := gopass.GetPasswd()
		if err != nil {
			return nil, err
		}

		if !IsEqualBytes(password, confirm) {
			return nil, errors.New("input password unmatched")
		}
	}

	return password, nil
}

func ShowAccountInfo(name string, password []byte) error {
	var err error
	password, err = GetPassword(password, false)
	if err != nil {
		return err
	}

	keyStore, err := walt.OpenKeystore(name, password)
	if err != nil {
		return err
	}

	// print header
	fmt.Printf("%-34s %-66s\n", "ADDRESS", "PUBLIC KEY")
	fmt.Println(strings.Repeat("-", 34), strings.Repeat("-", 66))

	// print account
	publicKey := keyStore.GetPublicKey()
	publicKeyBytes, _ := publicKey.EncodePoint(true)
	fmt.Printf("%-34s %-66s\n", keyStore.Address(), BytesToHexString(publicKeyBytes))
	// print divider line
	fmt.Println(strings.Repeat("-", 34), strings.Repeat("-", 66))

	return nil
}

func SelectAccount(wallet walt.Wallet) (string, error) {
	addrs, err := wallet.GetAddresses()
	if err != nil || len(addrs) == 0 {
		return "", errors.New("fail to load wallet addresses")
	}

	// only one address return it
	if len(addrs) == 1 {
		return addrs[0].Address, nil
	}

	// show accounts
	err = ShowAccounts(addrs, nil, wallet)
	if err != nil {
		return "", err
	}

	// select address by index input
	fmt.Println("Please input the address INDEX you want to use and press enter")

	index := -1
	for index == -1 {
		index = getInput(len(addrs))
	}

	return addrs[index].Address, nil
}

func ShowAccounts(addrs []*walt.Address, newAddr *Uint168, wallet walt.Wallet) error {
	// print header
	width, err := pb.GetTerminalWidth()
	if err != nil {
		return errors.New("get current ternimal width failed")
	}

	fmt.Println(strings.Repeat("-", width))
	currentHeight := wallet.CurrentHeight(walt.QueryHeightCode)
	for i, addr := range addrs {
		assetIDs, err := wallet.GetAssetIDs(addr.ProgramHash)
		if err != nil {
			return errors.New("get " + addr.Address + " assetIDs failed")
		}

		var format = "%-15s %-s\n"
		fmt.Printf("%-15s %d\n", "INDEX", i+1)
		fmt.Printf(format, "ADDRESS", addr.Address)
		for _, assetID := range assetIDs {
			available := big.NewInt(0)
			locked := big.NewInt(0)
			UTXOs, err := wallet.GetAddressUTXOs(addr.ProgramHash, assetID)
			if err != nil {
				return errors.New("get " + addr.Address + " UTXOs failed")
			}
			for _, utxo := range UTXOs {
				amountBigInt := big.NewInt(0)
				if utxo.AssetID.IsEqual(walt.SystemAssetId) {
					var amount Fixed64
					reader := bytes.NewReader(utxo.Amount)
					amount.Deserialize(reader)
					amountBigInt  = big.NewInt(amount.IntValue())
				} else {
					amountBigInt.SetBytes(utxo.Amount)
				}

				if utxo.LockTime < currentHeight {
					available.Add(available, amountBigInt)
				} else {
					locked.Add(locked, amountBigInt)
				}
			}
			fmt.Printf(format, "ASSETID", BytesToHexString(BytesReverse(assetID.Bytes())))
			if assetID.IsEqual(walt.SystemAssetId) {
				fmt.Printf(format, "  ├──BALANCE", Fixed64(available.Int64()).String())
				fmt.Printf(format, "  └──(LOCKED)", "("+Fixed64(locked.Int64()).String()+")")
			} else {
				fmt.Printf(format, "  ├──BALANCE", getValue(available))
				fmt.Printf(format, "  └──(LOCKED)", "("+getValue(locked)+")")
			}
		}
		fmt.Printf(format, "TYPE", addr.TypeName())
		fmt.Println(strings.Repeat("-", width))
	}

	return nil
}

func getValue( value *big.Int) string {
	var buff bytes.Buffer
	if value.Sign() == 0 {
		return "0"
	} else if value.Sign() < 0 {
		buff.WriteRune('-')
	}
	str := value.String()
	strLen := len(str)
	if strLen > 18 {
		buff.WriteString(str[:len(str) - 19 - 1])
		buff.WriteRune('.')
		buff.WriteString(str[len(str) - 19 - 1: len(str) - 1])
	}
	return buff.String()
}

func getInput(max int) int {
	fmt.Print("INPUT INDEX: ")
	input, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		fmt.Println("read input falied")
		return -1
	}

	// trim space
	input = strings.TrimSpace(input)

	index, err := strconv.ParseInt(input, 10, 32)
	if err != nil {
		fmt.Println("please input a positive integer")
		return -1
	}

	if int(index) > max {
		fmt.Println("INDEX should between 1 ~", max)
		return -1
	}

	return int(index) - 1
}
