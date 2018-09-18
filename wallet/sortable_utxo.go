package wallet

import (
	"sort"
	"bytes"
	"math/big"

	"github.com/elastos/Elastos.ELA.Utility/common"
)

type SortableUTXOs []*UTXO

func (utxos SortableUTXOs) Len() int      { return len(utxos) }
func (utxos SortableUTXOs) Swap(i, j int) { utxos[i], utxos[j] = utxos[j], utxos[i] }
func (utxos SortableUTXOs) Less(i, j int) bool {
	var amountI big.Int
	var amountJ big.Int
	if utxos[i].AssetID.IsEqual(SystemAssetId) {
		var amount common.Fixed64
		reader := bytes.NewReader(utxos[i].Amount)
		amount.Deserialize(reader)
		amountI  = *big.NewInt(amount.IntValue())
	} else {
		amountI.SetBytes(utxos[i].Amount)
	}
	if utxos[j].AssetID.IsEqual(SystemAssetId) {
		var amount common.Fixed64
		reader := bytes.NewReader(utxos[j].Amount)
		amount.Deserialize(reader)
		amountJ  = *big.NewInt(amount.IntValue())
	} else {
		amountJ.SetBytes(utxos[j].Amount)
	}


	if amountI.Cmp(&amountJ) > 0 {
		return false
	} else {
		return true
	}
}

func SortUTXOs(utxos []*UTXO) []*UTXO {
	sortableUTXOs := SortableUTXOs(utxos)
	sort.Sort(sortableUTXOs)
	return sortableUTXOs
}
