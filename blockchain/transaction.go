// Copyright (c) 2018 IoTeX
// This is an alpha (internal) release and is not suitable for production. This source code is provided ‘as is’ and no
// warranties are given as to title or non-infringement, merchantability or fitness for purpose and, to the extent
// permitted by law, all liability for your use of the code is disclaimed. This source code is governed by Apache
// License 2.0 that can be found in the LICENSE file.

package blockchain

import (
	"bytes"
	"crypto/rand"
	"fmt"

	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"golang.org/x/crypto/blake2b"

	cm "github.com/iotexproject/iotex-core/common"
	cp "github.com/iotexproject/iotex-core/crypto"
	"github.com/iotexproject/iotex-core/proto"
	"github.com/iotexproject/iotex-core/txvm"
)

const (
	// TxOutputPb fields

	// ValueSizeInBytes defines the size of value in byte units
	ValueSizeInBytes = 8
	// LockScriptSizeInBytes defines the size of lock script in byte units
	LockScriptSizeInBytes = 4

	// TxPb fields

	// VersionSizeInBytes defines the size of version in byte units
	VersionSizeInBytes = 4
	// NumTxInSizeInBytes defines the size of number of transaction inputs in byte units
	NumTxInSizeInBytes = 4
	// NumTxOutSizeInBytes defines the size of number of transaction outputs in byte units
	NumTxOutSizeInBytes = 4
	// LockTimeSizeInBytes defines the size of lock time in byte units
	LockTimeSizeInBytes = 4
)

// TxInput defines the transaction input protocol buffer
type TxInput = iproto.TxInputPb

// TxOutput defines the transaction output protocol buffer
type TxOutput struct {
	*iproto.TxOutputPb // embedded

	// below fields only used internally, not part of serialize/deserialize
	outIndex int32 // outIndex is needed when spending UTXO
}

// Tx defines the struct of transaction
// make sure the variable type and order of this struct is same as "type Tx" in blockchain.pb.go
type Tx struct {
	Version  uint32
	NumTxIn  uint32 // number of transaction input
	TxIn     []*TxInput
	NumTxOut uint32 // number of transaction output
	TxOut    []*TxOutput
	LockTime uint32 // UTXO to be locked until this time
}

// NewTxInput returns a TxInput instance
func NewTxInput(hash cp.Hash32B, index int32, unlock []byte, seq uint32) *TxInput {
	return &TxInput{
		hash[:],
		index,
		uint32(len(unlock)),
		unlock,
		seq}
}

// NewTxOutput returns a TxOutput instance
func NewTxOutput(amount uint64, index int32) *TxOutput {
	return &TxOutput{
		&iproto.TxOutputPb{amount, 0, nil},
		index}
}

// NewTx returns a Tx instance
func NewTx(version uint32, in []*TxInput, out []*TxOutput, lockTime uint32) *Tx {
	return &Tx{
		version,
		uint32(len(in)),
		in, uint32(len(out)),
		out,
		lockTime}
}

// Payee defines the struct of payee
type Payee struct {
	Address string
	Amount  uint64
}

// NewPayee returns a Payee instance
func NewPayee(address string, amount uint64) *Payee {
	return &Payee{address, amount}
}

// NewCoinbaseTx creates the coinbase transaction - a special type of transaction that does not require previously outputs.
func NewCoinbaseTx(toaddr string, amount uint64, data string) *Tx {
	if data == "" {
		randData := make([]byte, 20)
		_, err := rand.Read(randData)
		if err != nil {
			glog.Error(err)
			return nil
		}

		data = fmt.Sprintf("%x", randData)
	}

	txin := NewTxInput(cp.ZeroHash32B, -1, []byte(data), 0xffffffff)
	txout := CreateTxOutput(toaddr, amount)
	return NewTx(1, []*TxInput{txin}, []*TxOutput{txout}, 0)
}

// IsCoinbase checks if it is a coinbase transaction by checking if Vin is empty
func (tx *Tx) IsCoinbase() bool {
	return len(tx.TxIn) == 1 && len(tx.TxOut) == 1 && tx.TxIn[0].OutIndex == -1 && tx.TxIn[0].Sequence == 0xffffffff &&
		bytes.Compare(tx.TxIn[0].TxHash[:], cp.ZeroHash32B[:]) == 0
}

// TotalSize returns the total size of this transaction
func (tx *Tx) TotalSize() uint32 {
	size := uint32(VersionSizeInBytes + NumTxInSizeInBytes + NumTxOutSizeInBytes + LockTimeSizeInBytes)

	// add trnx input size
	for _, in := range tx.TxIn {
		size += in.TotalSize()
	}

	// add trnx output size
	for _, out := range tx.TxOut {
		size += out.TotalSize()
	}
	return size
}

// ByteStream returns a raw byte stream of trnx data
func (tx *Tx) ByteStream() []byte {
	stream := make([]byte, 4)
	cm.MachineEndian.PutUint32(stream, tx.Version)

	temp := make([]byte, 4)
	cm.MachineEndian.PutUint32(temp, tx.NumTxIn)
	stream = append(stream, temp...)

	// write all trnx input
	for _, txIn := range tx.TxIn {
		stream = append(stream, txIn.ByteStream()...)
	}

	cm.MachineEndian.PutUint32(temp, tx.NumTxOut)
	stream = append(stream, temp...)

	// write all trnx output
	for _, txOut := range tx.TxOut {
		stream = append(stream, txOut.ByteStream()...)
	}
	cm.MachineEndian.PutUint32(temp, tx.LockTime)
	stream = append(stream, temp...)

	return stream
}

// ConvertToTxPb creates a protobuf's Tx using type Tx
func (tx *Tx) ConvertToTxPb() *iproto.TxPb {
	pbOut := make([]*iproto.TxOutputPb, len(tx.TxOut))
	for i, out := range tx.TxOut {
		pbOut[i] = out.TxOutputPb
	}

	return &iproto.TxPb{
		tx.Version,
		tx.NumTxIn,
		tx.TxIn,
		tx.NumTxOut,
		pbOut,
		tx.LockTime}
}

// Serialize returns a serialized byte stream for the Tx
func (tx *Tx) Serialize() ([]byte, error) {
	return proto.Marshal(tx.ConvertToTxPb())
}

// ConvertFromTxPb converts a protobuf's Tx back to type Tx
func (tx *Tx) ConvertFromTxPb(pbTx *iproto.TxPb) {
	// set trnx fields
	tx.Version = pbTx.GetVersion()
	tx.NumTxIn = pbTx.GetNumTxIn()
	tx.NumTxOut = pbTx.GetNumTxOut()
	tx.LockTime = pbTx.GetLockTime()

	tx.TxIn = nil
	tx.TxIn = pbTx.TxIn

	tx.TxOut = nil
	tx.TxOut = make([]*TxOutput, len(pbTx.TxOut))
	for i, out := range pbTx.TxOut {
		tx.TxOut[i] = &TxOutput{out, int32(i)}
	}
}

// Deserialize parse the byte stream into the Tx
func (tx *Tx) Deserialize(buf []byte) error {
	pbTx := iproto.TxPb{}
	if err := proto.Unmarshal(buf, &pbTx); err != nil {
		panic(err)
	}

	tx.ConvertFromTxPb(&pbTx)
	return nil
}

// Hash returns the hash of the Tx
func (tx *Tx) Hash() cp.Hash32B {
	hash := blake2b.Sum256(tx.ByteStream())
	return blake2b.Sum256(hash[:])
}

//
// below are transaction output functions
//

// CreateTxOutput creates a new transaction output
func CreateTxOutput(toaddr string, value uint64) *TxOutput {
	out := NewTxOutput(value, 0)

	locks, err := txvm.PayToAddrScript(toaddr)
	if err != nil {
		glog.Error(err)
		return nil
	}
	out.LockScript = locks
	out.LockScriptSize = uint32(len(out.LockScript))

	return out
}

// IsLockedWithKey checks if the UTXO in output is locked with script
func (out *TxOutput) IsLockedWithKey(lockScript []byte) bool {
	if len(out.LockScript) < 23 {
		glog.Error("LockScript too short")
		return false
	}
	// TODO: avoid hard-coded extraction of public key hash
	return bytes.Compare(out.LockScript[3:23], lockScript) == 0
}

// TotalSize returns the total size of transaction output
func (out *TxOutput) TotalSize() uint32 {
	return ValueSizeInBytes + LockScriptSizeInBytes + uint32(out.LockScriptSize)
}

// ByteStream returns a raw byte stream of transaction output
func (out *TxOutput) ByteStream() []byte {
	stream := make([]byte, 8)
	cm.MachineEndian.PutUint64(stream, out.Value)

	temp := make([]byte, 4)
	cm.MachineEndian.PutUint32(temp, out.LockScriptSize)
	stream = append(stream, temp...)
	stream = append(stream, out.LockScript...)

	return stream
}
