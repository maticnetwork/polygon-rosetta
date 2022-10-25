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
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"math/big"
	"reflect"
	"strconv"
	"strings"

	"github.com/coinbase/rosetta-sdk-go/parser"
	"github.com/coinbase/rosetta-sdk-go/types"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/maticnetwork/polygon-rosetta/polygon"
	svcErrors "github.com/maticnetwork/polygon-rosetta/services/errors"
)

// ConstructionPreprocess implements the /construction/preprocess
// endpoint.
func (a *APIService) ConstructionPreprocess(
	ctx context.Context,
	request *types.ConstructionPreprocessRequest,
) (*types.ConstructionPreprocessResponse, *types.Error) {
	isContractCall := false
	if _, ok := request.Metadata["method_signature"]; ok {
		isContractCall = true
	}
	fromOp, toOp, err := matchTransferOperations(request.Operations, isContractCall)
	if err != nil {
		return nil, svcErrors.WrapErr(svcErrors.ErrUnclearIntent, err)
	}
	fromAdd := fromOp.Account.Address
	toAdd := toOp.Account.Address

	// Ensure valid from address
	checkFrom, ok := polygon.ChecksumAddress(fromAdd)
	if !ok {
		return nil, svcErrors.WrapErr(
			svcErrors.ErrInvalidAddress,
			fmt.Errorf("source address: %s is not a valid address", fromAdd),
		)
	}

	// Ensure valid to address
	checkTo, ok := polygon.ChecksumAddress(toAdd)
	if !ok {
		return nil, svcErrors.WrapErr(
			svcErrors.ErrInvalidAddress,
			fmt.Errorf("destination address: %s is not a valid address", toAdd),
		)
	}

	value := new(big.Int)
	value.SetString(toOp.Amount.Value, 10)
	preprocessOutputOptions := &options{
		From:  checkFrom,
		To:    checkTo,
		Value: value,
	}

	// Override nonce
	if v, ok := request.Metadata["nonce"]; ok {
		stringObj, ok := v.(string)
		if !ok {
			return nil, svcErrors.WrapErr(
				svcErrors.ErrInvalidNonce,
				fmt.Errorf("%s is not a valid nonce string", v),
			)
		}
		bigObj, ok := new(big.Int).SetString(stringObj, 10) //nolint:gomnd
		if !ok {
			return nil, svcErrors.WrapErr(
				svcErrors.ErrInvalidNonce,
				fmt.Errorf("%s is not a valid nonce", v),
			)
		}
		preprocessOutputOptions.Nonce = bigObj
	}

	// Override gas_tip
	if v, ok := request.Metadata["gas_tip"]; ok {
		stringObj, ok := v.(string)
		if !ok {
			return nil, svcErrors.WrapErr(
				svcErrors.ErrInvalidGasTip,
				fmt.Errorf("%s is not a valid gas_tip string", v),
			)
		}
		bigObj, ok := new(big.Int).SetString(stringObj, 10) //nolint:gomnd
		if !ok {
			return nil, svcErrors.WrapErr(
				svcErrors.ErrInvalidGasTip,
				fmt.Errorf("%s is not a valid gas_tip", v),
			)
		}
		preprocessOutputOptions.GasTip = bigObj
	}

	// Override gas_cap
	if v, ok := request.Metadata["gas_cap"]; ok {
		stringObj, ok := v.(string)
		if !ok {
			return nil, svcErrors.WrapErr(
				svcErrors.ErrInvalidGasCap,
				fmt.Errorf("%s is not a valid gas_cap string", v),
			)
		}
		bigObj, ok := new(big.Int).SetString(stringObj, 10) //nolint:gomnd
		if !ok {
			return nil, svcErrors.WrapErr(
				svcErrors.ErrInvalidGasCap,
				fmt.Errorf("%s is not a valid gas_cap", v),
			)
		}
		preprocessOutputOptions.GasCap = bigObj
	}

	// Override gas_limit
	if v, ok := request.Metadata["gas_limit"]; ok {
		stringObj, ok := v.(string)
		if !ok {
			return nil, svcErrors.WrapErr(
				svcErrors.ErrInvalidGasLimit,
				fmt.Errorf("%s is not a valid gas_limit string", v),
			)
		}
		bigObj, ok := new(big.Int).SetString(stringObj, 10) //nolint:gomnd
		if !ok {
			return nil, svcErrors.WrapErr(
				svcErrors.ErrInvalidGasLimit,
				fmt.Errorf("%s is not a valid gas_limit", v),
			)
		}
		preprocessOutputOptions.GasLimit = bigObj
	}

	// Only supports ERC20 transfers
	currency := fromOp.Amount.Currency
	if _, ok := request.Metadata["method_signature"]; !ok && !isNativeCurrency(currency) {
		tokenContractAddress, err := getTokenContractAddress(currency)
		if err != nil {
			return nil, svcErrors.WrapErr(svcErrors.ErrInvalidTokenContractAddress, err)
		}

		data, err := constructERC20TransferData(checkTo, value)
		if err != nil {
			return nil, svcErrors.WrapErr(svcErrors.ErrFetchFunctionSignatureMethodID, err)
		}

		preprocessOutputOptions.TokenAddress = tokenContractAddress
		preprocessOutputOptions.Data = data
		preprocessOutputOptions.Value = big.NewInt(0) // MATIC value is 0 when sending ERC20
	}

	if v, ok := request.Metadata["method_signature"]; ok {
		methodSigStringObj := v.(string)
		if !ok {
			return nil, svcErrors.WrapErr(
				svcErrors.ErrInvalidSignature,
				fmt.Errorf("%s is not a valid signature string", v),
			)
		}

		data, err := constructContractCallData(methodSigStringObj, request.Metadata["method_args"])
		if err != nil {
			return nil, svcErrors.WrapErr(svcErrors.ErrFetchFunctionSignatureMethodID, err)
		}
		preprocessOutputOptions.ContractAddress = checkTo
		preprocessOutputOptions.Data = data
		preprocessOutputOptions.MethodSignature = methodSigStringObj
		preprocessOutputOptions.MethodArgs = request.Metadata["method_args"]

	}

	marshaled, err := marshalJSONMap(preprocessOutputOptions)
	if err != nil {
		return nil, svcErrors.WrapErr(svcErrors.ErrUnableToParseIntermediateResult, err)
	}

	return &types.ConstructionPreprocessResponse{
		Options: marshaled,
	}, nil
}

// matchTransferOperations attempts to match a slice of operations with a `transfer`
// intent. This will match both Native token (Matic) and ERC20 tokens
func matchTransferOperations(operations []*types.Operation, isContractCall bool) (
	*types.Operation,
	*types.Operation,
	error,
) {
	valueOne, valueTwo := new(big.Int), new(big.Int)
	valueOne, ok := valueOne.SetString(operations[0].Amount.Value, 10)
	if !ok {
		log.Fatal("unable to convert valueOne to bigint")
	}
	valueTwo, ok = valueTwo.SetString(operations[1].Amount.Value, 10)
	if !ok {
		log.Fatal("unable to convert valueTwo to bigint")
	}
	if isContractCall && valueOne.BitLen() == 0 {
		if valueOne.Cmp(valueTwo) != 0 {
			return nil, nil, errors.New("for generic call both values should be zero")
		}
		descriptions := &parser.Descriptions{
			OperationDescriptions: []*parser.OperationDescription{
				{
					Type: polygon.CallOpType,
					Account: &parser.AccountDescription{
						Exists: true,
					},
					Amount: &parser.AmountDescription{
						Exists: true,
						Sign:   parser.AnyAmountSign,
					},
				},
				{
					Type: polygon.CallOpType,
					Account: &parser.AccountDescription{
						Exists: true,
					},
					Amount: &parser.AmountDescription{
						Exists: true,
						Sign:   parser.AnyAmountSign,
					},
				},
			},
			ErrUnmatched: true,
		}

		matches, err := parser.MatchOperations(descriptions, operations)
		if err != nil {
			return nil, nil, err
		}

		fromOp, _ := matches[0].First()
		toOp, _ := matches[1].First()

		// Manually validate currencies since we cannot rely on parser
		if fromOp.Amount.Currency == nil || toOp.Amount.Currency == nil {
			return nil, nil, errors.New("missing currency")
		}

		if !reflect.DeepEqual(fromOp.Amount.Currency, toOp.Amount.Currency) {
			return nil, nil, errors.New("from and to currencies are not equal")
		}

		return fromOp, toOp, nil

	}
	descriptions := &parser.Descriptions{
		OperationDescriptions: []*parser.OperationDescription{
			{
				Type: polygon.CallOpType,
				Account: &parser.AccountDescription{
					Exists: true,
				},
				Amount: &parser.AmountDescription{
					Exists: true,
					Sign:   parser.NegativeAmountSign,
				},
			},
			{
				Type: polygon.CallOpType,
				Account: &parser.AccountDescription{
					Exists: true,
				},
				Amount: &parser.AmountDescription{
					Exists: true,
					Sign:   parser.PositiveAmountSign,
				},
			},
		},
		ErrUnmatched: true,
	}

	matches, err := parser.MatchOperations(descriptions, operations)
	if err != nil {
		return nil, nil, err
	}

	fromOp, _ := matches[0].First()
	toOp, _ := matches[1].First()

	// Manually validate currencies since we cannot rely on parser
	if fromOp.Amount.Currency == nil || toOp.Amount.Currency == nil {
		return nil, nil, errors.New("missing currency")
	}

	if !reflect.DeepEqual(fromOp.Amount.Currency, toOp.Amount.Currency) {
		return nil, nil, errors.New("from and to currencies are not equal")
	}

	return fromOp, toOp, nil
}

// isNativeCurrency checks if the currency is the native currency
func isNativeCurrency(currency *types.Currency) bool {
	if currency == nil {
		return false
	}

	return reflect.DeepEqual(currency, polygon.Currency)
}

// getTokenContractAddress retrieves and validates the contract address
func getTokenContractAddress(currency *types.Currency) (string, error) {
	v, exists := currency.Metadata[TokenContractAddressKey]
	if !exists {
		return "", errors.New("missing token contract address")
	}

	tokenContractAddress, ok := v.(string)
	if !ok {
		return "", errors.New("token contract address is not a string")
	}

	checkTokenContractAddress, ok := polygon.ChecksumAddress(tokenContractAddress)
	if !ok {
		return "", errors.New("token contract address is not a valid address")
	}

	// TODO: verify token contract address actually exist and the Symbol matches
	return checkTokenContractAddress, nil
}

// constructERC20TransferData constructs the data field of a Polygon
// transaction, including the recipient address and the amount
func constructERC20TransferData(to string, value *big.Int) ([]byte, error) {
	methodID, err := erc20TransferMethodID()
	if err != nil {
		return nil, err
	}

	var data []byte
	data = append(data, methodID...)

	toAddress := common.HexToAddress(to)
	paddedToAddress := common.LeftPadBytes(toAddress.Bytes(), 32)
	data = append(data, paddedToAddress...)

	paddedAmount := common.LeftPadBytes(value.Bytes(), 32)
	data = append(data, paddedAmount...)

	return data, nil
}

// constructContractCallData constructs the data field of a Polygon
// transaction
func constructContractCallData(methodSig string, methodArgsGeneric interface{}) ([]byte, error) {
	data, sigErr := contractCallMethodID(methodSig)
	if sigErr != nil {
		return nil, sigErr
	}

	// switch on the type of the method args. method args can come in from json as either a string or list of strings
	switch methodArgs := methodArgsGeneric.(type) {

	// case 0: no method arguments, return the selector
	case nil:
		return data, nil

	// case 1: method args are pre-compiled ABI data. decode the hex and create the call data directly
	case string:
		methodArgs = strings.TrimPrefix(methodArgs, "0x")
		b, decErr := hex.DecodeString(methodArgs)
		if decErr != nil {
			return nil, fmt.Errorf("error decoding method args hex data: %w", decErr)
		}
		return append(data, b...), nil

	// case 2: method args are a list of interface{} which will be converted to string before encoding
	case []interface{}:
		var strList []string
		for i, genericVal := range methodArgs {
			strVal, isStrVal := genericVal.(string)
			if !isStrVal {
				return nil, fmt.Errorf("invalid method_args type at index %d: %T (must be a string)",
					i, genericVal,
				)
			}
			strList = append(strList, strVal)
		}
		return encodeMethodArgsStrings(data, methodSig, strList)

	// case 3: method args are encoded as a list of strings, which will be decoded
	case []string:
		return encodeMethodArgsStrings(data, methodSig, methodArgs)

	// case 4: there is no known way to decode the method args
	default:
		return nil, fmt.Errorf(
			"invalid method_args type, accepted values are []string and hex-encoded string."+
				" type received=%T value=%#v", methodArgsGeneric, methodArgsGeneric,
		)
	}
}

func encodeMethodArgsStrings(sigData []byte, methodSig string, methodArgs []string) ([]byte, error) {
	var arguments abi.Arguments
	var argumentsData []interface{}

	splitSigByLeadingParenthesis := strings.Split(methodSig, "(")
	if len(splitSigByLeadingParenthesis) < 2 {
		return nil, nil
	}
	splitSigByTrailingParenthesis := strings.Split(splitSigByLeadingParenthesis[1], ")")
	if len(splitSigByTrailingParenthesis) < 1 {
		return nil, nil
	}
	splitSigByComma := strings.Split(splitSigByTrailingParenthesis[0], ",")

	if len(splitSigByComma) != len(methodArgs) {
		return nil, errors.New("invalid method arguments")
	}

	for i, v := range splitSigByComma {
		typed, _ := abi.NewType(v, v, nil)
		argument := abi.Arguments{
			{
				Type: typed,
			},
		}

		arguments = append(arguments, argument...)
		var argData interface{}

		switch {
		case v == "address":
			{
				argData = common.HexToAddress(methodArgs[i])
			}
		case strings.HasPrefix(v, "uint") || strings.HasPrefix(v, "int"):
			{
				value := new(big.Int)
				value.SetString(methodArgs[i], 10)
				argData = value
			}
		case strings.HasPrefix(v, "bytes"):
			{
				value := [32]byte{}
				copy(value[:], []byte(methodArgs[i]))
				argData = value
			}
		case strings.HasPrefix(v, "string"):
			{
				argData = methodArgs[i]
			}
		case strings.HasPrefix(v, "bool"):
			{
				value, err := strconv.ParseBool(methodArgs[i])
				if err != nil {
					log.Fatal(err)
				}
				argData = value
			}

		}
		argumentsData = append(argumentsData, argData)
	}
	encData, _ := arguments.PackValues(argumentsData)
	return append(sigData, encData...), nil
}
