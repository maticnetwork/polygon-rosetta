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

	"github.com/maticnetwork/polygon-rosetta/configuration"
	"github.com/maticnetwork/polygon-rosetta/polygon"
	svcErrors "github.com/maticnetwork/polygon-rosetta/services/errors"
	"github.com/coinbase/rosetta-sdk-go/types"
	"github.com/ethereum/go-ethereum/common"
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

	var checkTokenContractAddress string
	gasLimit := polygon.TransferGasLimit
	to := checkTo
	// Only work for ERC20 transfer
	if len(input.TokenAddress) > 0 {
		checkTokenContractAddress, ok = polygon.ChecksumAddress(input.TokenAddress)
		if !ok {
			return nil, svcErrors.WrapErr(
				svcErrors.ErrInvalidAddress,
				fmt.Errorf("%s is not a valid address", input.TokenAddress),
			)
		}
		// Override the destination address to be the contract address
		to = checkTokenContractAddress

		var err *types.Error
		gasLimit, err = a.calculateGasLimit(ctx, checkFrom, checkTokenContractAddress, input.Data)
		if err != nil {
			return nil, err
		}
	}

	// TODO: Upgrade to use EIP1559
	gasPrice, err := a.client.SuggestGasPrice(ctx, input.GasPrice)
	if err != nil {
		return nil, svcErrors.WrapErr(svcErrors.ErrGeth, err)
	}

	metadata := &metadata{
		Nonce:    nonce,
		GasPrice: gasPrice,
		GasLimit: big.NewInt(int64(gasLimit)),
		Data:     input.Data,
		Value:    input.Value,
		To:       to,
	}

	metadataMap, err := marshalJSONMap(metadata)
	if err != nil {
		return nil, svcErrors.WrapErr(svcErrors.ErrUnableToParseIntermediateResult, err)
	}

	// Find suggested gas usage
	suggestedFee := metadata.GasPrice.Int64() * int64(gasLimit)

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

// calculatesGasLimit calculates the gasLimit for an ERC20 transfer
func (a *APIService) calculateGasLimit(
	ctx context.Context,
	from string,
	to string,
	data []byte,
) (uint64, *types.Error) {
	fromAddress := common.HexToAddress(from)
	toAddress := common.HexToAddress(to)
	gasLimit, err := a.client.EstimateGas(ctx, ethereum.CallMsg{
		From: fromAddress,
		To:   &toAddress,
		Data: data,
	})

	if err != nil {
		return 0, svcErrors.WrapErr(svcErrors.ErrGeth, err)
	}

	return gasLimit, nil
}
