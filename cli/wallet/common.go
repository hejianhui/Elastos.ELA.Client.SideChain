package wallet

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
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
			availableELA := Fixed64(0)
			lockedELA := Fixed64(0)
			availableToken := new(big.Int)
			lockedToken := new(big.Int)
			UTXOs, err := wallet.GetAddressUTXOs(addr.ProgramHash, assetID)
			if err != nil {
				return errors.New("get " + addr.Address + " UTXOs failed" + err.Error())
			}
			for _, utxo := range UTXOs {
				if utxo.LockTime < currentHeight {
					availableELA += *utxo.Amount
				} else {
					lockedELA += *utxo.Amount
				}
			}
			fmt.Printf(format, "ASSETID", BytesToHexString(BytesReverse(assetID.Bytes())))
			fmt.Printf(format, "  ├──ELABALANCE", availableELA.String())
			fmt.Printf(format, "  ├──(ELALOCKED)", "("+lockedELA.String()+")")
			fmt.Printf(format, "  ├──TOKENBALANCE", availableToken.String())
			fmt.Printf(format, "  └───TOKENLOCKED", lockedToken.String())
		}
		fmt.Printf(format, "TYPE", addr.TypeName())
		fmt.Println(strings.Repeat("-", width))
	}

	return nil
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
