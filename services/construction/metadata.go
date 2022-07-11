// Copyright 2021 Coinbase, Inc.
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
	"errors"
	"fmt"
	"math/big"
	"strconv"

	"github.com/ethereum/go-ethereum"

	"github.com/coinbase/rosetta-sdk-go/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/maticnetwork/polygon-rosetta/configuration"
	"github.com/maticnetwork/polygon-rosetta/polygon"
	svcErrors "github.com/maticnetwork/polygon-rosetta/services/errors"
)

// ConstructionMetadata implements the /construction/metadata endpoint.
func (a *APIService) ConstructionMetadata(
	ctx context.Context,
	request *types.ConstructionMetadataRequest,
) (*types.ConstructionMetadataResponse, *types.Error) {
	if a.config.Mode != configuration.Online {
		return nil, svcErrors.ErrUnavailableOffline
	}

	var input options
	if err := unmarshalJSONMap(request.Options, &input); err != nil {
		return nil, svcErrors.WrapErr(svcErrors.ErrUnableToParseIntermediateResult, err)
	}

	if len(input.From) == 0 {
		return nil, svcErrors.WrapErr(svcErrors.ErrInvalidAddress, errors.New("source address is not provided"))
	}

	if len(input.To) == 0 {
		return nil, svcErrors.WrapErr(svcErrors.ErrInvalidAddress, errors.New("destination address is not provided"))
	}

	checkFrom, ok := polygon.ChecksumAddress(input.From)
	if !ok {
		return nil, svcErrors.WrapErr(svcErrors.ErrInvalidAddress, fmt.Errorf("%s is not a valid address", input.From))
	}

	checkTo, ok := polygon.ChecksumAddress(input.To)
	if !ok {
		return nil, svcErrors.WrapErr(svcErrors.ErrInvalidAddress, fmt.Errorf("%s is not a valid address", input.To))
	}

	nonce, err := a.calculateNonce(ctx, input.Nonce, checkFrom)
	if err != nil {
		return nil, svcErrors.WrapErr(svcErrors.ErrGeth, err)
	}

	var gasLimit uint64
	if input.GasLimit == nil {
		// by default, initialize gasLimit to the TransferGasLimit
		gasLimit = polygon.TransferGasLimit
	} else {
		gasLimit = input.GasLimit.Uint64()
	}
	
	to := checkTo
	// Only work for ERC20 transfer
	if len(input.TokenAddress) > 0 {
		checkTokenContractAddress, ok := polygon.ChecksumAddress(input.TokenAddress)
		if !ok {
			return nil, svcErrors.WrapErr(
				svcErrors.ErrInvalidAddress,
				fmt.Errorf("%s is not a valid address", input.TokenAddress),
			)
		}
		// Override the destination address to be the contract address
		to = checkTokenContractAddress
		var err *types.Error
		gasLimit, err = a.calculateGasLimit(ctx, checkFrom, checkTokenContractAddress, input.Data, input.Value, input.GasLimit)
		if err != nil {
			return nil, err
		}
	}

	// Only work for Generic Contract calls
	if len(input.ContractAddress) > 0 {
		checkContractAddress, ok := polygon.ChecksumAddress(input.ContractAddress)
		if !ok {
			return nil, svcErrors.WrapErr(
				svcErrors.ErrInvalidAddress,
				fmt.Errorf("%s is not a valid address", input.ContractAddress),
			)
		}
		// Override the destination address to be the contract address
		to = checkContractAddress

		var err *types.Error
		gasLimit, err = a.calculateGasLimit(ctx, checkFrom, checkContractAddress, input.Data, input.Value, input.GasLimit)
		if err != nil {
			return nil, err
		}
	}

	header, err := a.client.BlockHeader(ctx, nil)
	if err != nil {
		return nil, svcErrors.WrapErr(svcErrors.ErrGeth, err)
	}

	gasTip, err := a.client.SuggestGasTipCap(ctx)
	if err != nil {
		return nil, svcErrors.WrapErr(svcErrors.ErrGeth, err)
	}

	// Ensure the gas tip is at least 30 gwei. This is the minimum gas price recommended by the Polygon team.
	// See https://forum.polygon.technology/t/recommended-min-gas-price-setting/7604 for additional context.
	// This minimum must be applied to the tip, not the cap to effectivley mitigate spam (since the tip goes to miners)
	minTip := big.NewInt(30000000000) // 30 gwei
	if minTip.Cmp(gasTip) == 1 {
		gasTip = minTip
	}

	var gasCap *big.Int
	if input.GasCap == nil {
		// Set default max fee to double the last base fee plus multiplied priority tip
		// to ensure tx is highly likely to go out in the next block.
		multiplier := big.NewInt(2)
		gasCap = new(big.Int).Add(gasTip, new(big.Int).Mul(header.BaseFee, multiplier))
	} else {
		gasCap = input.GasCap
	}

	metadata := &metadata{
		Nonce:           nonce,
		GasLimit:        gasLimit,
		GasCap:          gasCap,
		GasTip:          gasTip,
		Data:            input.Data,
		Value:           input.Value,
		To:              to,
		MethodSignature: input.MethodSignature,
		MethodArgs:      input.MethodArgs,
	}

	metadataMap, err := marshalJSONMap(metadata)
	if err != nil {
		return nil, svcErrors.WrapErr(svcErrors.ErrUnableToParseIntermediateResult, err)
	}

	// Find suggested gas usage. Note that this figure accounts for the minimum (30 gwei), and
	// is potentially double the block base fee.
	suggestedFee := gasCap.Int64() * int64(gasLimit)

	return &types.ConstructionMetadataResponse{
		Metadata: metadataMap,
		SuggestedFee: []*types.Amount{
			{
				Value:    strconv.FormatInt(suggestedFee, 10),
				Currency: polygon.Currency,
			},
		},
	}, nil
}

// calculateNonce will calculate the nonce for the from address if
// nonce is not provided
func (a *APIService) calculateNonce(
	ctx context.Context,
	nonceInput *big.Int,
	from string,
) (uint64, error) {
	if nonceInput == nil {
		nonce, err := a.client.PendingNonceAt(ctx, common.HexToAddress(from))
		if err != nil {
			return 0, err
		}
		return nonce, nil
	}
	return nonceInput.Uint64(), nil
}

// calculatesGasLimit calculates the gasLimit for an ERC20 transfer if
// gasLimit is not provided
func (a *APIService) calculateGasLimit(
	ctx context.Context,
	from string,
	to string,
	data []byte,
	value *big.Int,
	gasLimitInput *big.Int,
) (uint64, *types.Error) {
	if gasLimitInput != nil {
		return gasLimitInput.Uint64(), nil
	}

	fromAddress := common.HexToAddress(from)
	toAddress := common.HexToAddress(to)
	var v *big.Int
	if value != nil && value.Cmp(big.NewInt(0)) != 0 {
		v = value
	}
	gasLimit, err := a.client.EstimateGas(ctx, ethereum.CallMsg{
		From:  fromAddress,
		To:    &toAddress,
		Data:  data,
		Value: v,
	})

	if err != nil {
		return 0, svcErrors.WrapErr(svcErrors.ErrGeth, err)
	}

	return gasLimit, nil
}
