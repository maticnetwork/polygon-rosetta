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

	SuggestGasPrice(ctx context.Context, gasPrice *big.Int) (*big.Int, error)

	SendTransaction(ctx context.Context, tx *ethTypes.Transaction) error

	Call(
		ctx context.Context,
		request *types.CallRequest,
	) (*types.CallResponse, error)
}

// Nonce is a *big.Int so that its value can be checked against nil
// in MarshalJSON and ConstructionMetadata. If uint64 is used instead,
// its nil value will be 0 which is a valid nonce. This will cause
// ConstructionMetadata to make an extra call to eth_getTransactionCount
//
// Value here is MATIC. It will always be 0 for ERC20 tokens
type options struct {
	From         string   `json:"from"`
	Nonce        *big.Int `json:"nonce,omitempty"`
	Data         []byte   `json:"data,omitempty"`
	To           string   `json:"to"`
	TokenAddress string   `json:"token_address,omitempty"`
	Value        *big.Int `json:"value,omitempty"`
	GasPrice     *big.Int `json:"gas_price,omitempty"`
}

type optionsWire struct {
	From         string `json:"from"`
	Nonce        string `json:"nonce,omitempty"`
	Data         string `json:"data,omitempty"`
	To           string `json:"to"`
	TokenAddress string `json:"token_address,omitempty"`
	Value        string `json:"value,omitempty"`
	GasPrice     string `json:"gas_price,omitempty"`
}

func (o *options) MarshalJSON() ([]byte, error) {
	ow := &optionsWire{
		From: o.From,
		To:   o.To,
	}

	if o.Nonce != nil {
		ow.Nonce = hexutil.EncodeBig(o.Nonce)
	}

	if len(o.Data) > 0 {
		ow.Data = hexutil.Encode(o.Data)
	}

	if len(o.TokenAddress) > 0 {
		ow.TokenAddress = o.TokenAddress
	}

	if o.Value != nil {
		ow.Value = hexutil.EncodeBig(o.Value)
	}

	if o.GasPrice != nil {
		ow.GasPrice = hexutil.EncodeBig(o.GasPrice)
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

	if len(ow.GasPrice) > 0 {
		gasPrice, err := hexutil.DecodeBig(ow.GasPrice)
		if err != nil {
			return err
		}
		o.GasPrice = gasPrice
	}

	return nil
}

type metadata struct {
	Nonce    uint64   `json:"nonce"`
	GasPrice *big.Int `json:"gas_price"`
	GasLimit *big.Int `json:"gas_limit,omitempty"`
	Data     []byte   `json:"data,omitempty"`
	To       string   `json:"to,omitempty"`
	Value    *big.Int `json:"value,omitempty"`
}

type metadataWire struct {
	Nonce    string `json:"nonce"`
	GasPrice string `json:"gas_price"`
	GasLimit string `json:"gas_limit,omitempty"`
	Data     string `json:"data,omitempty"`
	To       string `json:"to,omitempty"`
	Value    string `json:"value,omitempty"`
}

func (m *metadata) MarshalJSON() ([]byte, error) {
	mw := &metadataWire{
		Nonce:    hexutil.Uint64(m.Nonce).String(),
		GasPrice: hexutil.EncodeBig(m.GasPrice),
		To:       m.To,
	}

	if m.GasLimit != nil {
		mw.GasLimit = hexutil.EncodeBig(m.GasLimit)
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

	gasPrice, err := hexutil.DecodeBig(mw.GasPrice)
	if err != nil {
		return err
	}

	m.GasPrice = gasPrice
	m.Nonce = nonce
	m.To = mw.To

	if len(mw.GasLimit) > 0 {
		gasLimit, err := hexutil.DecodeBig(mw.GasLimit)
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
	GasPrice *big.Int `json:"gas_price"`
	GasLimit uint64   `json:"gas_limit"`
	ChainID  *big.Int `json:"chain_id"`
}

type parseMetadataWire struct {
	Nonce    string `json:"nonce"`
	GasPrice string `json:"gas_price"`
	GasLimit string `json:"gas_limit"`
	ChainID  string `json:"chain_id"`
}

func (p *parseMetadata) MarshalJSON() ([]byte, error) {
	pmw := &parseMetadataWire{
		Nonce:    hexutil.Uint64(p.Nonce).String(),
		GasPrice: hexutil.EncodeBig(p.GasPrice),
		GasLimit: hexutil.Uint64(p.GasLimit).String(),
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
	GasPrice *big.Int `json:"gas_price"`
	GasLimit uint64   `json:"gas"`
	ChainID  *big.Int `json:"chain_id"`
}

type transactionWire struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Value    string `json:"value"`
	Data     string `json:"data"`
	Nonce    string `json:"nonce"`
	GasPrice string `json:"gas_price"`
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
		GasPrice: hexutil.EncodeBig(t.GasPrice),
		GasLimit: hexutil.EncodeUint64(t.GasLimit),
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

	gasPrice, err := hexutil.DecodeBig(tw.GasPrice)
	if err != nil {
		return err
	}

	gasLimit, err := hexutil.DecodeUint64(tw.GasLimit)
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
	t.GasPrice = gasPrice
	t.GasLimit = gasLimit
	t.ChainID = chainID
	t.GasPrice = gasPrice
	return nil
}
