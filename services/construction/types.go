// Copyright 2020 Coinbase, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package construction

import (
	"context"
	"encoding/json"
	"math/big"

	"github.com/ethereum/go-ethereum"

	"github.com/coinbase/rosetta-sdk-go/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethTypes "github.com/ethereum/go-ethereum/core/types"
)

// Client is used by the servicers to get block
// data and to submit transactions.
type Client interface {
	Status(context.Context) (
		*types.BlockIdentifier,
		int64,
		*types.SyncStatus,
		[]*types.Peer,
		error,
	)

	Block(
		context.Context,
		*types.PartialBlockIdentifier,
	) (*types.Block, error)

	Balance(
		context.Context,
		*types.AccountIdentifier,
		*types.PartialBlockIdentifier,
		[]*types.Currency,
	) (*types.AccountBalanceResponse, error)

	PendingNonceAt(context.Context, common.Address) (uint64, error)

	EstimateGas(ctx context.Context, msg ethereum.CallMsg) (uint64, error)

	BlockHeader(ctx context.Context, number *big.Int) (*ethTypes.Header, error)

	SendTransaction(ctx context.Context, tx *ethTypes.Transaction) error

	SuggestGasTipCap(ctx context.Context) (*big.Int, error)

	Call(
		ctx context.Context,
		request *types.CallRequest,
	) (*types.CallResponse, error)
}

// Nonce and GasLimit are a *big.Int so that their values can be checked against nil
// in MarshalJSON and ConstructionMetadata. For nonce, if uint64 is used instead,
// its nil value will be 0 which is a valid nonce. This will cause
// ConstructionMetadata to make an extra call to eth_getTransactionCount
//
// Value here is MATIC. It will always be 0 for ERC20 tokens
type options struct {
	From            string      `json:"from"`
	Nonce           *big.Int    `json:"nonce,omitempty"`
	Data            []byte      `json:"data,omitempty"`
	To              string      `json:"to"`
	TokenAddress    string      `json:"token_address,omitempty"`
	ContractAddress string      `json:"contract_address,omitempty"`
	Value           *big.Int    `json:"value,omitempty"`
	GasLimit        *big.Int    `json:"gas_limit,omitempty"`
	GasCap          *big.Int    `json:"gas_cap,omitempty"`
	GasTip          *big.Int    `json:"gas_tip,omitempty"`
	MethodSignature string      `json:"method_signature,omitempty"`
	MethodArgs      interface{} `json:"method_args,omitempty"`
}

type optionsWire struct {
	From            string      `json:"from"`
	Nonce           string      `json:"nonce,omitempty"`
	Data            string      `json:"data,omitempty"`
	To              string      `json:"to"`
	TokenAddress    string      `json:"token_address,omitempty"`
	ContractAddress string      `json:"contract_address,omitempty"`
	Value           string      `json:"value,omitempty"`
	GasLimit        string      `json:"gas_limit,omitempty"`
	GasCap          string      `json:"gas_cap,omitempty"`
	GasTip          string      `json:"gas_tip,omitempty"`
	MethodSignature string      `json:"method_signature,omitempty"`
	MethodArgs      interface{} `json:"method_args,omitempty"`
}

func (o *options) MarshalJSON() ([]byte, error) {
	ow := &optionsWire{
		From:            o.From,
		To:              o.To,
		ContractAddress: o.ContractAddress,
		MethodSignature: o.MethodSignature,
		MethodArgs:      o.MethodArgs,
		TokenAddress:    o.TokenAddress,
	}

	if o.Nonce != nil {
		ow.Nonce = hexutil.EncodeBig(o.Nonce)
	}

	if len(o.Data) > 0 {
		ow.Data = hexutil.Encode(o.Data)
	}

	if o.Value != nil {
		ow.Value = hexutil.EncodeBig(o.Value)
	}

	if o.GasLimit != nil {
		ow.GasLimit = hexutil.EncodeBig(o.GasLimit)
	}

	if o.GasCap != nil {
		ow.GasCap = hexutil.EncodeBig(o.GasCap)
	}

	if o.GasTip != nil {
		ow.GasTip = hexutil.EncodeBig(o.GasTip)
	}

	return json.Marshal(ow)
}

func (o *options) UnmarshalJSON(data []byte) error {
	var ow optionsWire
	if err := json.Unmarshal(data, &ow); err != nil {
		return err
	}
	o.From = ow.From
	o.To = ow.To
	o.TokenAddress = ow.TokenAddress
	o.ContractAddress = ow.ContractAddress
	o.MethodSignature = ow.MethodSignature
	o.MethodArgs = ow.MethodArgs

	if len(ow.Nonce) > 0 {
		nonce, err := hexutil.DecodeBig(ow.Nonce)
		if err != nil {
			return err
		}
		o.Nonce = nonce
	}

	if len(ow.Data) > 0 {
		owData, err := hexutil.Decode(ow.Data)
		if err != nil {
			return err
		}
		o.Data = owData
	}

	if len(ow.Value) > 0 {
		value, err := hexutil.DecodeBig(ow.Value)
		if err != nil {
			return err
		}
		o.Value = value
	}

	if len(ow.GasLimit) > 0 {
		gasLimit, err := hexutil.DecodeBig(ow.GasLimit)
		if err != nil {
			return err
		}
		o.GasLimit = gasLimit
	}

	if len(ow.GasCap) > 0 {
		gasCap, err := hexutil.DecodeBig(ow.GasCap)
		if err != nil {
			return err
		}
		o.GasCap = gasCap
	}

	if len(ow.GasTip) > 0 {
		gasTip, err := hexutil.DecodeBig(ow.GasTip)
		if err != nil {
			return err
		}
		o.GasTip = gasTip
	}

	return nil
}

type metadata struct {
	Nonce           uint64      `json:"nonce"`
	GasCap          *big.Int    `json:"gas_cap"`
	GasTip          *big.Int    `json:"gas_tip"`
	GasLimit        uint64      `json:"gas_limit,omitempty"`
	Data            []byte      `json:"data,omitempty"`
	To              string      `json:"to,omitempty"`
	Value           *big.Int    `json:"value,omitempty"`
	MethodSignature string      `json:"method_signature,omitempty"`
	MethodArgs      interface{} `json:"method_args,omitempty"`
}

type metadataWire struct {
	Nonce           string      `json:"nonce"`
	GasCap          string      `json:"gas_cap"`
	GasTip          string      `json:"gas_tip"`
	GasLimit        string      `json:"gas_limit,omitempty"`
	Data            string      `json:"data,omitempty"`
	To              string      `json:"to,omitempty"`
	Value           string      `json:"value,omitempty"`
	MethodSignature string      `json:"method_signature,omitempty"`
	MethodArgs      interface{} `json:"method_args,omitempty"`
}

func (m *metadata) MarshalJSON() ([]byte, error) {
	mw := &metadataWire{
		Nonce:           hexutil.Uint64(m.Nonce).String(),
		GasCap:          hexutil.EncodeBig(m.GasCap),
		GasTip:          hexutil.EncodeBig(m.GasTip),
		To:              m.To,
		MethodSignature: m.MethodSignature,
		MethodArgs:      m.MethodArgs,
	}

	if m.GasLimit > 0 {
		mw.GasLimit = hexutil.Uint64(m.GasLimit).String()
	}

	if len(m.Data) > 0 {
		mw.Data = hexutil.Encode(m.Data)
	}

	if m.Value != nil {
		mw.Value = hexutil.EncodeBig(m.Value)
	}

	return json.Marshal(mw)
}

func (m *metadata) UnmarshalJSON(data []byte) error {
	var mw metadataWire
	if err := json.Unmarshal(data, &mw); err != nil {
		return err
	}

	nonce, err := hexutil.DecodeUint64(mw.Nonce)
	if err != nil {
		return err
	}

	gasCap, err := hexutil.DecodeBig(mw.GasCap)
	if err != nil {
		return err
	}

	gasTip, err := hexutil.DecodeBig(mw.GasTip)
	if err != nil {
		return err
	}

	m.GasCap = gasCap
	m.GasTip = gasTip
	m.Nonce = nonce
	m.To = mw.To
	m.MethodSignature = mw.MethodSignature
	m.MethodArgs = mw.MethodArgs

	if len(mw.GasLimit) > 0 {
		gasLimit, err := hexutil.DecodeUint64(mw.GasLimit)
		if err != nil {
			return err
		}
		m.GasLimit = gasLimit
	}

	if len(mw.Data) > 0 {
		mwData, err := hexutil.Decode(mw.Data)
		if err != nil {
			return err
		}
		m.Data = mwData
	}

	if len(mw.Value) > 0 {
		value, err := hexutil.DecodeBig(mw.Value)
		if err != nil {
			return err
		}
		m.Value = value
	}

	return nil
}

type parseMetadata struct {
	Nonce    uint64   `json:"nonce"`
	GasLimit uint64   `json:"gas_limit"`
	GasCap   *big.Int `json:"gas_cap"`
	GasTip   *big.Int `json:"gas_tip"`
	ChainID  *big.Int `json:"chain_id"`
}

type parseMetadataWire struct {
	Nonce    string `json:"nonce"`
	GasLimit string `json:"gas_limit"`
	GasCap   string `json:"gas_cap"`
	GasTip   string `json:"gas_tip"`
	ChainID  string `json:"chain_id"`
}

func (p *parseMetadata) MarshalJSON() ([]byte, error) {
	pmw := &parseMetadataWire{
		Nonce:    hexutil.Uint64(p.Nonce).String(),
		GasLimit: hexutil.Uint64(p.GasLimit).String(),
		GasCap:   hexutil.EncodeBig(p.GasCap),
		GasTip:   hexutil.EncodeBig(p.GasTip),
		ChainID:  hexutil.EncodeBig(p.ChainID),
	}

	return json.Marshal(pmw)
}

type transaction struct {
	From     string   `json:"from"`
	To       string   `json:"to"`
	Value    *big.Int `json:"value"`
	Data     []byte   `json:"data"`
	Nonce    uint64   `json:"nonce"`
	GasCap   *big.Int `json:"max_fee_per_gas"`
	GasTip   *big.Int `json:"max_priority_fee_per_gas"`
	GasLimit uint64   `json:"gas"`
	ChainID  *big.Int `json:"chain_id"`
}

type transactionWire struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Value    string `json:"value"`
	Data     string `json:"data"`
	Nonce    string `json:"nonce"`
	GasCap   string `json:"max_fee_per_gas"`
	GasTip   string `json:"max_priority_fee_per_gas"`
	GasLimit string `json:"gas"`
	ChainID  string `json:"chain_id"`
}

func (t *transaction) MarshalJSON() ([]byte, error) {
	tw := &transactionWire{
		From:     t.From,
		To:       t.To,
		Value:    hexutil.EncodeBig(t.Value),
		Data:     hexutil.Encode(t.Data),
		Nonce:    hexutil.EncodeUint64(t.Nonce),
		GasLimit: hexutil.EncodeUint64(t.GasLimit),
		GasCap:   hexutil.EncodeBig(t.GasCap),
		GasTip:   hexutil.EncodeBig(t.GasTip),
		ChainID:  hexutil.EncodeBig(t.ChainID),
	}

	return json.Marshal(tw)
}

func (t *transaction) UnmarshalJSON(data []byte) error {

	var tw transactionWire
	if err := json.Unmarshal(data, &tw); err != nil {
		return err
	}

	value, err := hexutil.DecodeBig(tw.Value)
	if err != nil {
		return err
	}

	twData, err := hexutil.Decode(tw.Data)
	if err != nil {
		return err
	}

	nonce, err := hexutil.DecodeUint64(tw.Nonce)
	if err != nil {
		return err
	}

	gasLimit, err := hexutil.DecodeUint64(tw.GasLimit)
	if err != nil {
		return err
	}

	gasCap, err := hexutil.DecodeBig(tw.GasCap)
	if err != nil {
		return err
	}
	gasTip, err := hexutil.DecodeBig(tw.GasTip)
	if err != nil {
		return err
	}

	chainID, err := hexutil.DecodeBig(tw.ChainID)
	if err != nil {
		return err
	}

	t.From = tw.From
	t.To = tw.To
	t.Value = value
	t.Data = twData
	t.Nonce = nonce
	t.GasLimit = gasLimit
	t.GasCap = gasCap
	t.GasTip = gasTip
	t.ChainID = chainID
	return nil
}
