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

package polygon

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"reflect"
	"sort"
	"strconv"
	"time"

	RosettaTypes "github.com/coinbase/rosetta-sdk-go/types"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	EthTypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/eth/tracers"
	// "github.com/ethereum/go-ethereum/p2p" // HOTFIX: temorarily unused
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/rpc"
	BorTypes "github.com/maticnetwork/polygon-rosetta/polygon/types"
	"golang.org/x/sync/semaphore"

	"github.com/maticnetwork/polygon-rosetta/polygon/utilities/artifacts"
)

const (
	gethHTTPTimeout = 120 * time.Second

	maxTraceConcurrency  = int64(16) // nolint:gomnd
	semaphoreTraceWeight = int64(1)  // nolint:gomnd

	// TODO: refactor consts
	// ERC20 Standard Definition for the Transfer Event Logs Topics
	erc20TransferEventLogTopics = "Transfer(address,address,uint256)"

	// While parsing ERC20 ops, we will ignore any event logs that we think are an ERC20 transfer
	// that do not contain 3 topics and who's `data` field is not a single 32 byte hex string
	// representing the amount of the transfer
	numTopicsERC20Transfer = 3

	// eip1559TxType is the EthTypes.Transaction.Type() value that indicates this transaction
	// follows EIP-1559.
	eip1559TxType = 2
)

// Client allows for querying a set of specific Ethereum endpoints in an
// idempotent manner. Client relies on the eth_*, debug_*, and admin_*
// methods and on the graphql endpoint.
//
// Client borrows HEAVILY from https://github.com/ethereum/go-ethereum/tree/master/ethclient.
type Client struct {
	p  *params.ChainConfig
	tc *tracers.TraceCallConfig

	c JSONRPC
	g GraphQL

	currencyFetcher CurrencyFetcher

	traceSemaphore *semaphore.Weighted

	skipAdminCalls bool

	burntContract map[string]string
}

// ClientConfig holds asset config information
type ClientConfig struct {
	URL            string
	ChainConfig    *params.ChainConfig
	SkipAdminCalls bool
	Headers        []*HTTPHeader
	BurntContract  map[string]string
}

// NewClient creates a Client that from the provided url and params.
func NewClient(cfg *ClientConfig) (*Client, error) {
	c, err := rpc.DialHTTPWithClient(cfg.URL, &http.Client{
		Timeout: gethHTTPTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: unable to dial node", err)
	}

	for _, header := range cfg.Headers {
		c.SetHeader(header.Key, header.Value)
	}

	tc, err := loadTraceConfig()
	if err != nil {
		return nil, fmt.Errorf("%w: unable to load trace config", err)
	}

	g, err := newGraphQLClient(cfg.URL, cfg.Headers)
	if err != nil {
		return nil, fmt.Errorf("%w: unable to create GraphQL client", err)
	}

	currencyFetcher, err := newERC20CurrencyFetcher(g)
	if err != nil {
		return nil, fmt.Errorf("%w: unable to create CurrencyFetcher", err)
	}

	return &Client{
		p:               cfg.ChainConfig,
		tc:              tc,
		c:               c,
		g:               g,
		currencyFetcher: currencyFetcher,
		traceSemaphore:  semaphore.NewWeighted(maxTraceConcurrency),
		skipAdminCalls:  cfg.SkipAdminCalls,
		burntContract:   cfg.BurntContract,
	}, nil
}

// Close shuts down the RPC client connection.
func (ec *Client) Close() {
	ec.c.Close()
}

// Status returns geth status information
// for determining node healthiness.
func (ec *Client) Status(ctx context.Context) (
	*RosettaTypes.BlockIdentifier,
	int64,
	*RosettaTypes.SyncStatus,
	[]*RosettaTypes.Peer,
	error,
) {
	header, err := ec.BlockHeader(ctx, nil)
	if err != nil {
		return nil, -1, nil, nil, err
	}

	progress, err := ec.syncProgress(ctx)
	if err != nil {
		return nil, -1, nil, nil, err
	}

	var syncStatus *RosettaTypes.SyncStatus
	if progress != nil {
		currentIndex := int64(progress.CurrentBlock)
		targetIndex := int64(progress.HighestBlock)

		syncStatus = &RosettaTypes.SyncStatus{
			CurrentIndex: &currentIndex,
			TargetIndex:  &targetIndex,
		}
	}

	// HOTFIX: temporarily removing this as it's unclear there is a current need
	// peers, err := ec.peers(ctx)
	// if err != nil {
	// 	return nil, -1, nil, nil, err
	// }

	emptyPeerSlice := []*RosettaTypes.Peer{}
	return &RosettaTypes.BlockIdentifier{
			Hash:  header.Hash().Hex(),
			Index: header.Number.Int64(),
		},
		convertTime(header.Time),
		syncStatus,
		// peers, // HOTFIX: removed temporarily
		emptyPeerSlice,
		nil
}

// PendingNonceAt returns the account nonce of the given account in the pending state.
// This is the nonce that should be used for the next transaction.
func (ec *Client) PendingNonceAt(ctx context.Context, account common.Address) (uint64, error) {
	var result hexutil.Uint64
	err := ec.c.CallContext(ctx, &result, "eth_getTransactionCount", account, "pending")
	return uint64(result), err
}

// SuggestGasTipCap retrieves the currently suggested gas tip cap after 1559 to
// allow a timely execution of a transaction.
func (ec *Client) SuggestGasTipCap(ctx context.Context) (*big.Int, error) {
	var hex hexutil.Big
	if err := ec.c.CallContext(ctx, &hex, "eth_maxPriorityFeePerGas"); err != nil {
		return nil, err
	}
	return (*big.Int)(&hex), nil
}

// EstimateGas retrieves the currently gas limit
func (ec *Client) EstimateGas(ctx context.Context, msg ethereum.CallMsg) (uint64, error) {
	var hex hexutil.Uint64
	err := ec.c.CallContext(ctx, &hex, "eth_estimateGas", toCallArg(msg))
	if err != nil {
		return 0, err
	}
	return uint64(hex), nil
}

// Peers retrieves all peers of the node.
// currently not using
// func (ec *Client) peers(ctx context.Context) ([]*RosettaTypes.Peer, error) {
// 	var info []*p2p.PeerInfo

// 	if ec.skipAdminCalls {
// 		return []*RosettaTypes.Peer{}, nil
// 	}

// 	if err := ec.c.CallContext(ctx, &info, "admin_peers"); err != nil {
// 		return nil, err
// 	}

// 	peers := make([]*RosettaTypes.Peer, len(info))
// 	for i, peerInfo := range info {
// 		peers[i] = &RosettaTypes.Peer{
// 			PeerID: peerInfo.ID,
// 			Metadata: map[string]interface{}{
// 				"name":      peerInfo.Name,
// 				"enode":     peerInfo.Enode,
// 				"caps":      peerInfo.Caps,
// 				"enr":       peerInfo.ENR,
// 				"protocols": peerInfo.Protocols,
// 			},
// 		}
// 	}

// 	return peers, nil
// }

// SendTransaction injects a signed transaction into the pending pool for execution.
//
// If the transaction was a contract creation use the TransactionReceipt method to get the
// contract address after the transaction has been mined.
func (ec *Client) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	data, err := rlp.EncodeToBytes(tx)
	if err != nil {
		return err
	}
	// We have to remove the first two bytes otherwise DynamicFeeTxs will not send:
	// https://ethereum.stackexchange.com/questions/124447/eth-sendrawtransaction-with-dynamicfeetx-returns-expected-input-list-for-types
	return ec.c.CallContext(ctx, nil, "eth_sendRawTransaction", hexutil.Encode(data[2:]))
}

func toBlockNumArg(number *big.Int) string {
	if number == nil {
		return "latest"
	}
	pending := big.NewInt(-1)
	if number.Cmp(pending) == 0 {
		return "pending"
	}
	return hexutil.EncodeBig(number)
}

// Block returns a populated block at the *RosettaTypes.PartialBlockIdentifier.
// If neither the hash or index is populated in the *RosettaTypes.PartialBlockIdentifier,
// the current block is returned.
func (ec *Client) Block(
	ctx context.Context,
	blockIdentifier *RosettaTypes.PartialBlockIdentifier,
) (*RosettaTypes.Block, error) {
	if blockIdentifier != nil {
		if blockIdentifier.Hash != nil {
			return ec.getParsedBlock(ctx, "eth_getBlockByHash", *blockIdentifier.Hash, true)
		}

		if blockIdentifier.Index != nil {
			return ec.getParsedBlock(ctx, "eth_getBlockByNumber", toBlockNumArg(big.NewInt(*blockIdentifier.Index)), true)
		}
	}

	return ec.getParsedBlock(ctx, "eth_getBlockByNumber", toBlockNumArg(nil), true)
}

// BlockHeader returns a block header from the current canonical chain. If number is
// nil, the latest known header is returned.
func (ec *Client) BlockHeader(ctx context.Context, number *big.Int) (*types.Header, error) {
	var head *types.Header
	err := ec.c.CallContext(ctx, &head, "eth_getBlockByNumber", toBlockNumArg(number), false)
	if err == nil && head == nil {
		return nil, ethereum.NotFound
	}

	return head, err
}

type rpcBlock struct {
	Hash         common.Hash      `json:"hash"`
	Transactions []rpcTransaction `json:"transactions"`
	UncleHashes  []common.Hash    `json:"uncles"`
}

func (ec *Client) getBlock(
	ctx context.Context,
	blockMethod string,
	args ...interface{},
) (
	*types.Block,
	[]*loadedTransaction,
	error,
) {
	var raw json.RawMessage
	err := ec.c.CallContext(ctx, &raw, blockMethod, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: block fetch failed", err)
	} else if len(raw) == 0 {
		return nil, nil, ethereum.NotFound
	}

	// Decode header and transactions
	var head types.Header
	var body rpcBlock
	if err := json.Unmarshal(raw, &head); err != nil {
		return nil, nil, err
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, nil, err
	}

	blockAuthor, err := ec.blockAuthor(ctx, head.Number.Int64())
	if err != nil {
		return nil, nil, fmt.Errorf("%w: could not get block author for %x", err, body.Hash[:])
	}

	receipts, err := ec.getBlockReceipts(ctx, body.Hash, body.Transactions)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: could not get receipts for %x", err, body.Hash[:])
	}

	// Get block traces (not possible to make idempotent block transaction trace requests)
	//
	// We fetch traces last because we want to avoid limiting the number of other
	// block-related data fetches we perform concurrently (we limit the number of
	// concurrent traces that are computed to 16 to avoid overwhelming geth).
	var traces []*rpcCall
	var rawTraces []*rpcRawCall
	var addTraces bool
	if head.Number.Int64() != GenesisBlockIndex { // not possible to get traces at genesis
		addTraces = true
		traces, rawTraces, err = ec.getBlockTraces(ctx, body.Hash)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: could not get traces for %x", err, body.Hash[:])
		}
	}

	// Convert all txs to loaded txs
	txs := make([]*types.Transaction, len(body.Transactions))
	loadedTxs := make([]*loadedTransaction, len(body.Transactions))
	for i, tx := range body.Transactions {
		txs[i] = tx.tx
		receipt := receipts[i]
		gasUsed := new(big.Int).SetUint64(receipt.GasUsed)
		gasPrice, err := effectiveGasPrice(txs[i], head.BaseFee)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: failure getting gas price", err)
		}
		loadedTxs[i] = tx.LoadedTransaction()
		loadedTxs[i].Transaction = txs[i]
		loadedTxs[i].FeeAmount = new(big.Int).Mul(gasUsed, gasPrice)
		if head.BaseFee != nil { // EIP-1559
			loadedTxs[i].FeeBurned = new(big.Int).Mul(gasUsed, head.BaseFee)
		} else {
			loadedTxs[i].FeeBurned = nil
		}
		loadedTxs[i].Author = MustChecksum(blockAuthor.Address)
		loadedTxs[i].Receipt = receipt

		// Continue if calls does not exist (occurs at genesis)
		if !addTraces {
			continue
		}

		// State sync tx has no traces, generate traces from the receipt logs instead
		if *tx.TxHash == BorTypes.GetDerivedBorTxHash(BorTypes.BorReceiptKey(head.Number.Uint64(), body.Hash)) {
			loadedTxs[i].Trace, loadedTxs[i].RawTrace = getStateSyncTraces(receipt)
		} else {
			loadedTxs[i].Trace = traces[i].Result
			loadedTxs[i].RawTrace = rawTraces[i].Result
		}
	}

	uncles := []*types.Header{} // no uncles in polygon
	return types.NewBlockWithHeader(&head).WithBody(txs, uncles), loadedTxs, nil
}

// effectiveGasPrice returns the price of gas charged to this transaction to be included in the
// block.
func effectiveGasPrice(tx *EthTypes.Transaction, baseFee *big.Int) (*big.Int, error) {
	if tx.Type() != eip1559TxType {
		return tx.GasPrice(), nil
	}
	// For EIP-1559 the gas price is determined by the base fee & miner tip instead
	// of the tx-specified gas price.
	tip, err := tx.EffectiveGasTip(baseFee)
	if err != nil {
		return nil, err
	}
	return new(big.Int).Add(tip, baseFee), nil
}

func getStateSyncTraces(receipt *types.Receipt) (*Call, json.RawMessage) {
	// Parent call is empty, there can be multiple TokenDeposited events which will be added as sub calls
	trace := &Call{
		Type:    "CALL",
		From:    common.Address{},
		To:      common.Address{},
		Value:   big.NewInt(0),
		GasUsed: big.NewInt(0),
		Calls:   []*Call{},
	}
	// leave rawTrace empty as we don't actually use it
	rawTrace := json.RawMessage("{}")

	// Get TokenDeposited events from the receipt Logs and convert to expected call traces
	for _, log := range receipt.Logs {
		// 0xec3a... is the topic hash for the token deposited event
		// TokenDeposited (index_topic_1 address rootToken, index_topic_2 address childToken,
		//                 index_topic_3 address user, uint256 amount, uint256 depositCount)
		if log.Topics[0].Hex() == "0xec3afb067bce33c5a294470ec5b29e6759301cd3928550490c6d48816cdc2f5d" {
			childTokenAddress := common.HexToAddress(log.Topics[2].Hex())
			toAddress := common.HexToAddress(log.Topics[3].Hex())
			// We only care about MATIC deposits for now. 0x1010 is the matic token address
			if childTokenAddress.Hex() == "0x0000000000000000000000000000000000001010" {
				// Data is amount + depositCount. We don't care about deposit count so just convert first 32 bytes to bigint
				value, _ := big.NewInt(0).SetString(hex.EncodeToString(log.Data[0:32]), 16)
				trace.Calls = append(trace.Calls, &Call{
					Type:  "CALL",
					From:  childTokenAddress,
					To:    toAddress,
					Value: value,
					// GasPrice is always zero so GasUsed is irrelevant
					GasUsed: big.NewInt(0),
				})
			}
		}
	}
	return trace, rawTrace
}

func (ec *Client) getBlockTraces(
	ctx context.Context,
	blockHash common.Hash,
) ([]*rpcCall, []*rpcRawCall, error) {
	if err := ec.traceSemaphore.Acquire(ctx, semaphoreTraceWeight); err != nil {
		return nil, nil, err
	}
	defer ec.traceSemaphore.Release(semaphoreTraceWeight)

	var calls []*rpcCall
	var rawCalls []*rpcRawCall
	var raw json.RawMessage
	err := ec.c.CallContext(ctx, &raw, "debug_traceBlockByHash", blockHash, ec.tc)
	if err != nil {
		return nil, nil, err
	}

	// Decode []*rpcCall
	if err := json.Unmarshal(raw, &calls); err != nil {
		return nil, nil, err
	}

	// Decode []*rpcRawCall
	if err := json.Unmarshal(raw, &rawCalls); err != nil {
		return nil, nil, err
	}

	return calls, rawCalls, nil
}

func (ec *Client) getBlockReceipts(
	ctx context.Context,
	blockHash common.Hash,
	txs []rpcTransaction,
) ([]*types.Receipt, error) {
	receipts := make([]*types.Receipt, len(txs))
	if len(txs) == 0 {
		return receipts, nil
	}

	reqs := make([]rpc.BatchElem, len(txs))
	for i := range reqs {
		reqs[i] = rpc.BatchElem{
			Method: "eth_getTransactionReceipt",
			Args:   []interface{}{txs[i].TxHash.Hex()},
			Result: &receipts[i],
		}
	}
	if err := ec.c.BatchCallContext(ctx, reqs); err != nil {
		return nil, err
	}
	for i := range reqs {
		if reqs[i].Error != nil {
			return nil, reqs[i].Error
		}
		if receipts[i] == nil {
			return nil, fmt.Errorf("got empty receipt for %x", txs[i].tx.Hash().Hex())
		}

		if receipts[i].BlockHash != blockHash {
			return nil, fmt.Errorf(
				"%w: expected block hash %s for transaction but got %s",
				ErrBlockOrphaned,
				blockHash.Hex(),
				receipts[i].BlockHash.Hex(),
			)
		}
	}

	return receipts, nil
}

type rpcCall struct {
	Result *Call `json:"result"`
}

type rpcRawCall struct {
	Result json.RawMessage `json:"result"`
}

// Call is an Ethereum debug trace.
type Call struct {
	Type         string         `json:"type"`
	From         common.Address `json:"from"`
	To           common.Address `json:"to"`
	Value        *big.Int       `json:"value"`
	GasUsed      *big.Int       `json:"gasUsed"`
	Revert       bool
	ErrorMessage string  `json:"error"`
	Calls        []*Call `json:"calls"`
}

type flatCall struct {
	Type         string         `json:"type"`
	From         common.Address `json:"from"`
	To           common.Address `json:"to"`
	Value        *big.Int       `json:"value"`
	GasUsed      *big.Int       `json:"gasUsed"`
	Revert       bool
	ErrorMessage string `json:"error"`
}

func (t *Call) flatten() *flatCall {
	return &flatCall{
		Type:         t.Type,
		From:         t.From,
		To:           t.To,
		Value:        t.Value,
		GasUsed:      t.GasUsed,
		Revert:       t.Revert,
		ErrorMessage: t.ErrorMessage,
	}
}

// UnmarshalJSON is a custom unmarshaler for Call.
func (t *Call) UnmarshalJSON(input []byte) error {
	type CustomTrace struct {
		Type         string         `json:"type"`
		From         common.Address `json:"from"`
		To           common.Address `json:"to"`
		Value        *hexutil.Big   `json:"value"`
		GasUsed      *hexutil.Big   `json:"gasUsed"`
		Revert       bool
		ErrorMessage string  `json:"error"`
		Calls        []*Call `json:"calls"`
	}
	var dec CustomTrace
	if err := json.Unmarshal(input, &dec); err != nil {
		return err
	}

	t.Type = dec.Type
	t.From = dec.From
	t.To = dec.To
	if dec.Value != nil {
		t.Value = (*big.Int)(dec.Value)
	} else {
		t.Value = new(big.Int)
	}
	if dec.GasUsed != nil {
		t.GasUsed = (*big.Int)(dec.Value)
	} else {
		t.GasUsed = new(big.Int)
	}
	if dec.ErrorMessage != "" {
		// Any error surfaced by the decoder means that the transaction
		// has reverted.
		t.Revert = true
	}
	t.ErrorMessage = dec.ErrorMessage
	t.Calls = dec.Calls
	return nil
}

// flattenTraces recursively flattens all traces.
func flattenTraces(data *Call, flattened []*flatCall) []*flatCall {
	results := append(flattened, data.flatten())
	for _, child := range data.Calls {
		// Ensure all children of a reverted call
		// are also reverted!
		if data.Revert {
			child.Revert = true

			// Copy error message from parent
			// if child does not have one
			if len(child.ErrorMessage) == 0 {
				child.ErrorMessage = data.ErrorMessage
			}
		}

		children := flattenTraces(child, flattened)
		results = append(results, children...)
	}
	return results
}

// containsTopic is a helper function that goes through a Log's Topics to see if it includes
// a given topic string
// TODO: create and move to separate utilities package
func containsTopic(log *types.Log, topic string) bool {
	for _, t := range log.Topics {
		hex := t.Hex()
		if hex == topic {
			return true
		}
	}
	return false
}

func buildGraphqlCallQuery(blockQuery string, contractAddress string, encodedData string) string {
	return fmt.Sprintf(`{
					block(%s){
						hash
						number
						call(data:{
							to:"%s"
							data:"%s"
						})
						{
							data
							status
							gasUsed
						}
					}
				}`, blockQuery, contractAddress, encodedData)
}

// erc20TokenOps returns all ERC2O-related *RosettaTypes.Operation for a given tx.Receipt.
func (ec *Client) erc20TokenOps(
	ctx context.Context,
	receipt *types.Receipt,
	startIndex int,
) ([]*RosettaTypes.Operation, error) {
	ops := []*RosettaTypes.Operation{}
	var status string
	if receipt.Status == 1 {
		status = SuccessStatus
	} else {
		status = FailureStatus
	}

	keccak := crypto.Keccak256([]byte(erc20TransferEventLogTopics))
	encodedTransferMethod := hexutil.Encode(keccak)

	for _, receiptLog := range receipt.Logs {
		// If this isn't an ERC20 transfer, skip
		if !containsTopic(receiptLog, encodedTransferMethod) {
			continue
		}

		if len(receiptLog.Topics) != numTopicsERC20Transfer {
			continue
		}

		value := new(big.Int).SetBytes(receiptLog.Data)

		// If value <= 0, skip to the next receiptLog. Otherwise, proceed to generate the debit + credit operations.
		if value.Cmp(big.NewInt(0)) < 1 {
			continue
		}

		contractAddress := receiptLog.Address.String()
		_, ok := ChecksumAddress(contractAddress)
		if !ok {
			return nil, fmt.Errorf("%s is not a valid address", contractAddress)
		}

		fromAddress := common.HexToAddress(receiptLog.Topics[1].Hex()).String()
		_, ok = ChecksumAddress(fromAddress)
		if !ok {
			return nil, fmt.Errorf("%s is not a valid address", fromAddress)
		}

		toAddress := common.HexToAddress(receiptLog.Topics[2].Hex()).String()
		_, ok = ChecksumAddress(toAddress)
		if !ok {
			return nil, fmt.Errorf("%s is not a valid address", toAddress)
		}
		
		// HOTFIX: not fetching currency details
		// currency, err := ec.currencyFetcher.fetchCurrency(ctx, contractAddress)

		// If an error is encountered while fetching currency details, return a default value and let the client handle it.
		// if err != nil {
		// 	log.Print(fmt.Sprintf("error while fetching currency details for currency: %s", contractAddress), err)
		// 	currency = &RosettaTypes.Currency{
		// 		Symbol:   defaultERC20Symbol,
		// 		Decimals: defaultERC20Decimals,
		// 		Metadata: map[string]interface{}{
		// 			ContractAddressKey: contractAddress,
		// 		},
		// 	}
		// }

		var currencyMap map[string]*RosettaTypes.Currency
		if ec.p.ChainID == big.NewInt(137) { // mainnet
			currencyMap = map[string]*RosettaTypes.Currency{
				"0x2791Bca1f2de4661ED88A30C99A7a9449Aa84174": {
					Symbol:   "USDC",
					Decimals: 6,
					Metadata: map[string]interface{}{
						ContractAddressKey: "0x2791Bca1f2de4661ED88A30C99A7a9449Aa84174",
					},
				},
				"0x7ceB23fD6bC0adD59E62ac25578270cFf1b9f619": {
					Symbol:   "WETH",
					Decimals: 18,
					Metadata: map[string]interface{}{
						ContractAddressKey: "0x7ceB23fD6bC0adD59E62ac25578270cFf1b9f619",
					},
				},
			}
		} else if ec.p.ChainID == big.NewInt(80001) { // mumbai
			currencyMap = map[string]*RosettaTypes.Currency{
				"0x0FA8781a83E46826621b3BC094Ea2A0212e71B23": {
					Symbol:   "USDC",
					Decimals: 6,
					Metadata: map[string]interface{}{
						ContractAddressKey: "0x0FA8781a83E46826621b3BC094Ea2A0212e71B23",
					},
				},
				"0xA6FA4fB5f76172d178d61B04b0ecd319C5d1C0aa": {
					Symbol:   "WETH",
					Decimals: 18,
					Metadata: map[string]interface{}{
						ContractAddressKey: "0xA6FA4fB5f76172d178d61B04b0ecd319C5d1C0aa",
					},
				},
			}
		}

		defaultCurrency := &RosettaTypes.Currency{
			Symbol:   defaultERC20Symbol,
			Decimals: defaultERC20Decimals,
			Metadata: map[string]interface{}{
				ContractAddressKey: contractAddress,
			},
		}

		currency := currencyMap[contractAddress]

		// if we don't care about this currency, set to defaultCurrency
		if currency == nil {
			currency = defaultCurrency
		}

		fromOp := &RosettaTypes.Operation{
			OperationIdentifier: &RosettaTypes.OperationIdentifier{
				Index: int64(len(ops) + startIndex),
			},
			Type:   PaymentOpType,
			Status: RosettaTypes.String(status),
			Account: &RosettaTypes.AccountIdentifier{
				Address: fromAddress,
			},
			Amount: &RosettaTypes.Amount{
				Value:    new(big.Int).Neg(value).String(),
				Currency: currency,
			},
		}

		ops = append(ops, fromOp)

		lastOpIndex := ops[len(ops)-1].OperationIdentifier.Index
		toOp := &RosettaTypes.Operation{
			OperationIdentifier: &RosettaTypes.OperationIdentifier{
				Index: lastOpIndex + 1,
			},
			RelatedOperations: []*RosettaTypes.OperationIdentifier{
				{
					Index: lastOpIndex,
				},
			},
			Type:   PaymentOpType,
			Status: RosettaTypes.String(status),
			Account: &RosettaTypes.AccountIdentifier{
				Address: toAddress,
			},
			Amount: &RosettaTypes.Amount{
				Value:    value.String(),
				Currency: currency,
			},
		}

		ops = append(ops, toOp)
	}

	return ops, nil
}

// traceOps returns all *RosettaTypes.Operation for a given
// array of flattened traces.
func traceOps(calls []*flatCall, startIndex int) []*RosettaTypes.Operation { // nolint: gocognit
	var ops []*RosettaTypes.Operation
	if len(calls) == 0 {
		return ops
	}

	destroyedAccounts := map[string]*big.Int{}
	for _, trace := range calls {
		// Handle partial transaction success
		metadata := map[string]interface{}{}
		opStatus := SuccessStatus
		if trace.Revert {
			opStatus = FailureStatus
			metadata["error"] = trace.ErrorMessage
		}

		var zeroValue bool
		if trace.Value.Sign() == 0 {
			zeroValue = true
		}

		// Skip all 0 value CallType operations (TODO: make optional to include)
		//
		// We can't continue here because we may need to adjust our destroyed
		// accounts map if a CallType operation resurrects an account.
		shouldAdd := true
		if zeroValue && CallType(trace.Type) {
			shouldAdd = false
		}

		// Checksum addresses
		from := MustChecksum(trace.From.String())
		to := MustChecksum(trace.To.String())

		if shouldAdd {
			fromOp := &RosettaTypes.Operation{
				OperationIdentifier: &RosettaTypes.OperationIdentifier{
					Index: int64(len(ops) + startIndex),
				},
				Type:   trace.Type,
				Status: RosettaTypes.String(opStatus),
				Account: &RosettaTypes.AccountIdentifier{
					Address: from,
				},
				Amount: &RosettaTypes.Amount{
					Value:    new(big.Int).Neg(trace.Value).String(),
					Currency: Currency,
				},
				Metadata: metadata,
			}
			if zeroValue {
				fromOp.Amount = nil
			} else {
				_, destroyed := destroyedAccounts[from]
				if destroyed && opStatus == SuccessStatus {
					destroyedAccounts[from] = new(big.Int).Sub(destroyedAccounts[from], trace.Value)
				}
			}

			ops = append(ops, fromOp)
		}

		// Add to destroyed accounts if SELFDESTRUCT
		// and overwrite existing balance.
		if trace.Type == SelfDestructOpType {
			destroyedAccounts[from] = new(big.Int)

			// If destination of of SELFDESTRUCT is self,
			// we should skip. In the EVM, the balance is reset
			// after the balance is increased on the destination
			// so this is a no-op.
			if from == to {
				continue
			}
		}

		// Skip empty to addresses (this may not
		// actually occur but leaving it as a
		// sanity check)
		if len(trace.To.String()) == 0 {
			continue
		}

		// If the account is resurrected, we remove it from
		// the destroyed accounts map.
		if CreateType(trace.Type) {
			delete(destroyedAccounts, to)
		}

		if shouldAdd {
			lastOpIndex := ops[len(ops)-1].OperationIdentifier.Index
			toOp := &RosettaTypes.Operation{
				OperationIdentifier: &RosettaTypes.OperationIdentifier{
					Index: lastOpIndex + 1,
				},
				RelatedOperations: []*RosettaTypes.OperationIdentifier{
					{
						Index: lastOpIndex,
					},
				},
				Type:   trace.Type,
				Status: RosettaTypes.String(opStatus),
				Account: &RosettaTypes.AccountIdentifier{
					Address: to,
				},
				Amount: &RosettaTypes.Amount{
					Value:    trace.Value.String(),
					Currency: Currency,
				},
				Metadata: metadata,
			}
			if zeroValue {
				toOp.Amount = nil
			} else {
				_, destroyed := destroyedAccounts[to]
				if destroyed && opStatus == SuccessStatus {
					destroyedAccounts[to] = new(big.Int).Add(destroyedAccounts[to], trace.Value)
				}
			}

			ops = append(ops, toOp)
		}
	}

	// Zero-out all destroyed accounts that are removed
	// during transaction finalization.
	for acct, val := range destroyedAccounts {
		if val.Sign() == 0 {
			continue
		}

		if val.Sign() < 0 {
			log.Fatalf("negative balance for suicided account %s: %s\n", acct, val.String())
		}

		ops = append(ops, &RosettaTypes.Operation{
			OperationIdentifier: &RosettaTypes.OperationIdentifier{
				Index: ops[len(ops)-1].OperationIdentifier.Index + 1,
			},
			Type:   DestructOpType,
			Status: RosettaTypes.String(SuccessStatus),
			Account: &RosettaTypes.AccountIdentifier{
				Address: acct,
			},
			Amount: &RosettaTypes.Amount{
				Value:    new(big.Int).Neg(val).String(),
				Currency: Currency,
			},
		})
	}

	return ops
}

type txExtraInfo struct {
	BlockNumber *string         `json:"blockNumber,omitempty"`
	BlockHash   *common.Hash    `json:"blockHash,omitempty"`
	From        *common.Address `json:"from,omitempty"`
	TxHash      *common.Hash    `json:"hash,omitempty"`
}

type rpcTransaction struct {
	tx *types.Transaction
	txExtraInfo
}

func (tx *rpcTransaction) UnmarshalJSON(msg []byte) error {
	if err := json.Unmarshal(msg, &tx.tx); err != nil {
		return err
	}
	return json.Unmarshal(msg, &tx.txExtraInfo)
}

func (tx *rpcTransaction) LoadedTransaction() *loadedTransaction {
	ethTx := &loadedTransaction{
		Transaction: tx.tx,
		From:        tx.txExtraInfo.From,
		BlockNumber: tx.txExtraInfo.BlockNumber,
		BlockHash:   tx.txExtraInfo.BlockHash,
		TxHash:      tx.TxHash,
	}
	return ethTx
}

type loadedTransaction struct {
	Transaction *types.Transaction
	From        *common.Address
	BlockNumber *string
	BlockHash   *common.Hash
	TxHash      *common.Hash // may not equal Transaction.Hash() due to state sync indicator
	FeeAmount   *big.Int
	FeeBurned   *big.Int // nil if no fees were burned
	Author      string
	Status      bool

	Trace    *Call
	RawTrace json.RawMessage
	Receipt  *types.Receipt
}

func (ec *Client) feeOps(tx *loadedTransaction, block *EthTypes.Block) []*RosettaTypes.Operation {
	if tx.FeeAmount.Cmp(new(big.Int)) == 0 {
		// This can happen for state sync transactions
		return []*RosettaTypes.Operation{}
	}
	var minerEarnedAmount *big.Int
	if tx.FeeBurned == nil {
		minerEarnedAmount = tx.FeeAmount
	} else {
		minerEarnedAmount = new(big.Int).Sub(tx.FeeAmount, tx.FeeBurned)
	}
	rOps := []*RosettaTypes.Operation{
		{
			OperationIdentifier: &RosettaTypes.OperationIdentifier{
				Index: 0,
			},
			Type:   FeeOpType,
			Status: RosettaTypes.String(SuccessStatus),
			Account: &RosettaTypes.AccountIdentifier{
				Address: MustChecksum(tx.From.String()),
			},
			Amount: &RosettaTypes.Amount{
				Value:    new(big.Int).Neg(minerEarnedAmount).String(),
				Currency: Currency,
			},
		},
		{
			OperationIdentifier: &RosettaTypes.OperationIdentifier{
				Index: 1,
			},
			RelatedOperations: []*RosettaTypes.OperationIdentifier{
				{
					Index: 0,
				},
			},
			Type:   FeeOpType,
			Status: RosettaTypes.String(SuccessStatus),
			Account: &RosettaTypes.AccountIdentifier{
				Address: MustChecksum(tx.Author),
			},
			Amount: &RosettaTypes.Amount{
				Value:    minerEarnedAmount.String(),
				Currency: Currency,
			},
		},
	}

	if tx.FeeBurned == nil {
		return rOps
	}

	// Burnt fees, if any, need to go to the burn contract.
	burntContract := ec.CalculateBurntContract(block.NumberU64())
	debitOp := &RosettaTypes.Operation{
		OperationIdentifier: &RosettaTypes.OperationIdentifier{
			Index: 2,
		},
		Type:   FeeOpType,
		Status: RosettaTypes.String(SuccessStatus),
		Account: &RosettaTypes.AccountIdentifier{
			Address: MustChecksum(tx.From.String()),
		},
		Amount: &RosettaTypes.Amount{
			Value:    new(big.Int).Neg(tx.FeeBurned).String(),
			Currency: Currency,
		},
	}
	creditOp := &RosettaTypes.Operation{
		OperationIdentifier: &RosettaTypes.OperationIdentifier{
			Index: 3,
		},
		RelatedOperations: []*RosettaTypes.OperationIdentifier{
			{
				Index: 2,
			},
		},
		Type:   FeeOpType,
		Status: RosettaTypes.String(SuccessStatus),
		Account: &RosettaTypes.AccountIdentifier{
			Address: MustChecksum(burntContract),
		},
		Amount: &RosettaTypes.Amount{
			Value:    tx.FeeBurned.String(),
			Currency: Currency,
		},
	}
	return append(rOps, debitOp, creditOp)
}

// transactionReceipt returns the receipt of a transaction by transaction hash.
// Note that the receipt is not available for pending transactions.
func (ec *Client) transactionReceipt(
	ctx context.Context,
	txHash common.Hash,
) (*types.Receipt, error) {
	var r *types.Receipt
	err := ec.c.CallContext(ctx, &r, "eth_getTransactionReceipt", txHash)
	if err == nil {
		if r == nil {
			return nil, ethereum.NotFound
		}
	}

	return r, err
}

func (ec *Client) getParsedBlock(
	ctx context.Context,
	blockMethod string,
	args ...interface{},
) (
	*RosettaTypes.Block,
	error,
) {
	block, loadedTransactions, err := ec.getBlock(ctx, blockMethod, args...)
	if err != nil {
		return nil, fmt.Errorf("%w: could not get block", err)
	}

	blockIdentifier := &RosettaTypes.BlockIdentifier{
		Hash:  block.Hash().String(),
		Index: block.Number().Int64(),
	}

	parentBlockIdentifier := blockIdentifier
	if blockIdentifier.Index != GenesisBlockIndex {
		parentBlockIdentifier = &RosettaTypes.BlockIdentifier{
			Hash:  block.ParentHash().Hex(),
			Index: blockIdentifier.Index - 1,
		}
	}

	txs, err := ec.populateTransactions(ctx, blockIdentifier, block, loadedTransactions)
	if err != nil {
		return nil, err
	}

	return &RosettaTypes.Block{
		BlockIdentifier:       blockIdentifier,
		ParentBlockIdentifier: parentBlockIdentifier,
		Timestamp:             convertTime(block.Time()),
		Transactions:          txs,
	}, nil
}

func convertTime(time uint64) int64 {
	return int64(time) * 1000
}

func (ec *Client) populateTransactions(
	ctx context.Context,
	blockIdentifier *RosettaTypes.BlockIdentifier,
	block *EthTypes.Block,
	loadedTransactions []*loadedTransaction,
) ([]*RosettaTypes.Transaction, error) {
	transactions := make([]*RosettaTypes.Transaction, len(block.Transactions()))
	for i, tx := range loadedTransactions {
		transaction, err := ec.populateTransaction(ctx, tx, block)
		if err != nil {
			return nil, fmt.Errorf("%w: cannot parse %s", err, tx.Transaction.Hash().Hex())
		}

		transactions[i] = transaction
	}

	return transactions, nil
}

func (ec *Client) populateTransaction(
	ctx context.Context,
	tx *loadedTransaction,
	block *EthTypes.Block,
) (*RosettaTypes.Transaction, error) {
	ops := []*RosettaTypes.Operation{}

	// Compute fee operations
	feeOps := ec.feeOps(tx, block)
	ops = append(ops, feeOps...)

	// Compute tx operations via tx.Receipt logs for ERC20 transfers
	erc20TokenOps, err := ec.erc20TokenOps(ctx, tx.Receipt, len(ops))
	if err != nil {
		return nil, err
	}

	ops = append(ops, erc20TokenOps...)

	// Compute trace operations
	traces := flattenTraces(tx.Trace, []*flatCall{})

	traceOps := traceOps(traces, len(ops))
	ops = append(ops, traceOps...)

	// Marshal receipt and trace data
	// TODO: replace with marshalJSONMap (used in `services`)
	receiptBytes, err := tx.Receipt.MarshalJSON()
	if err != nil {
		return nil, err
	}

	var receiptMap map[string]interface{}
	if err := json.Unmarshal(receiptBytes, &receiptMap); err != nil {
		return nil, err
	}

	var traceMap map[string]interface{}
	if err := json.Unmarshal(tx.RawTrace, &traceMap); err != nil {
		return nil, err
	}

	populatedTransaction := &RosettaTypes.Transaction{
		TransactionIdentifier: &RosettaTypes.TransactionIdentifier{
			Hash: tx.TxHash.Hex(),
		},
		Operations: ops,
		Metadata: map[string]interface{}{
			"gas_limit": hexutil.EncodeUint64(tx.Transaction.Gas()),
			"gas_price": hexutil.EncodeBig(tx.Transaction.GasPrice()),
			"receipt":   receiptMap,
			"trace":     traceMap,
		},
	}

	return populatedTransaction, nil
}

func (ec *Client) blockAuthor(ctx context.Context, blockIndex int64) (*RosettaTypes.AccountIdentifier, error) {
	// Genesis block has no validator, manually set nil address
	if blockIndex == GenesisBlockIndex {
		return &RosettaTypes.AccountIdentifier{
			Address: "0x0000000000000000000000000000000000000000",
		}, nil
	}
	var authorAddress string
	if err := ec.c.CallContext(ctx, &authorAddress, "bor_getAuthor", fmt.Sprintf("0x%x", blockIndex)); err != nil {
		return nil, err
	}
	return &RosettaTypes.AccountIdentifier{
		Address: authorAddress,
	}, nil
}

type rpcProgress struct {
	StartingBlock hexutil.Uint64
	CurrentBlock  hexutil.Uint64
	HighestBlock  hexutil.Uint64
	PulledStates  hexutil.Uint64
	KnownStates   hexutil.Uint64
}

// syncProgress retrieves the current progress of the sync algorithm. If there's
// no sync currently running, it returns nil.
func (ec *Client) syncProgress(ctx context.Context) (*ethereum.SyncProgress, error) {
	var raw json.RawMessage
	if err := ec.c.CallContext(ctx, &raw, "eth_syncing"); err != nil {
		return nil, err
	}

	var syncing bool
	if err := json.Unmarshal(raw, &syncing); err == nil {
		return nil, nil // Not syncing (always false)
	}

	var progress rpcProgress
	if err := json.Unmarshal(raw, &progress); err != nil {
		return nil, err
	}

	return &ethereum.SyncProgress{
		StartingBlock: uint64(progress.StartingBlock),
		CurrentBlock:  uint64(progress.CurrentBlock),
		HighestBlock:  uint64(progress.HighestBlock),
		PulledStates:  uint64(progress.PulledStates),
		KnownStates:   uint64(progress.KnownStates),
	}, nil
}

type graphqlBalance struct {
	Errors []struct {
		Message string   `json:"message"`
		Path    []string `json:"path"`
	} `json:"errors"`
	Data struct {
		Block struct {
			Hash    string `json:"hash"`
			Number  int64  `json:"number"`
			Account struct {
				Balance string `json:"balance"`
				Nonce   string `json:"transactionCount"`
				Code    string `json:"code"`
			} `json:"account"`
		} `json:"block"`
	} `json:"data"`
}

// decodeHexData is a helper function that accepts a fully formed hex string
// (including the 0x prefix) and returns a `big.Int`.
func decodeHexData(data string) (*big.Int, error) {
	decoded, ok := new(big.Int).SetString(data[2:], 16)
	if !ok {
		return nil, fmt.Errorf(
			"could not extract data from %s",
			data,
		)
	}

	return decoded, nil
}

func buildGraphqlBalanceQuery(blockQuery string, address string) string {
	return fmt.Sprintf(`{
			block(%s){
				hash
				number
				account(address:"%s"){
					balance
					transactionCount
					code
				}
			}
		}`, blockQuery, address)
}

// Balance returns the balance of a *RosettaTypes.AccountIdentifier
// at a *RosettaTypes.PartialBlockIdentifier.
//
// Note: If the currencies field is populated, only balances for the specified currencies
// will be returned. If not populated, the native balance (MATIC) will be returned.
// See: https://www.rosetta-api.org/docs/models/AccountBalanceRequest.html
// For each specified ERC20 token, we make a graphql call to its respective contract address
// in order to query the account's balance.
//
// We must use graphql to get the balance atomically (the
// rpc method for balance does not allow for querying
// by block hash nor return the block hash where
// the balance was fetched).
func (ec *Client) Balance(
	ctx context.Context,
	account *RosettaTypes.AccountIdentifier,
	block *RosettaTypes.PartialBlockIdentifier,
	currencies []*RosettaTypes.Currency,
) (*RosettaTypes.AccountBalanceResponse, error) {
	blockQuery := ""
	if block != nil {
		if block.Hash != nil {
			blockQuery = fmt.Sprintf(`hash: "%s"`, *block.Hash)
		}
		if block.Hash == nil && block.Index != nil {
			blockQuery = fmt.Sprintf("number: %d", *block.Index)
		}
	}

	// TODO: explicitly handle nonsuccessful status codes
	result, err := ec.g.Query(ctx, buildGraphqlBalanceQuery(blockQuery, account.Address))
	if err != nil {
		return nil, err
	}

	var bal graphqlBalance
	if err := json.Unmarshal([]byte(result), &bal); err != nil {
		return nil, err
	}

	if len(bal.Errors) > 0 {
		return nil, errors.New(RosettaTypes.PrintStruct(bal.Errors))
	}

	balance, err := decodeHexData(bal.Data.Block.Account.Balance)
	if err != nil {
		return nil, err
	}
	nonce, err := decodeHexData(bal.Data.Block.Account.Nonce)
	if err != nil {
		return nil, err
	}

	nativeBalance := &RosettaTypes.Amount{
		Value:    balance.String(),
		Currency: Currency,
	}

	balances := []*RosettaTypes.Amount{}

	// Pack the given method name to conform the ABI. Method call's data
	// will consist of method_id, args0, arg1, ... argN. Method id consists
	// of 4 bytes and arguments are all 32 bytes.
	erc20Data, err := artifacts.ERC20ABI.Pack("balanceOf", common.HexToAddress(account.Address))
	if err != nil {
		return nil, err
	}

	encodedErc20Data := hexutil.Encode(erc20Data)

	for _, curr := range currencies {
		if reflect.DeepEqual(curr, Currency) {
			balances = append(balances, nativeBalance)
			continue
		}

		contractAddress := fmt.Sprintf("%s", curr.Metadata[ContractAddressKey])
		_, ok := ChecksumAddress(contractAddress)
		if !ok {
			return nil, fmt.Errorf("invalid contract address %s", contractAddress)
		}

		// graphql spec:
		// https://github.com/ethereum/go-ethereum/blob/90987db7334c1d10eb866ca550efedb66dea8a20/graphql/schema.go#L236-L255
		result, err := ec.g.Query(ctx, buildGraphqlCallQuery(blockQuery, contractAddress, encodedErc20Data))
		if err != nil {
			return nil, err
		}

		var bal graphqlCallResponse
		if err := json.Unmarshal([]byte(result), &bal); err != nil {
			return nil, err
		}

		if len(bal.Errors) > 0 {
			return nil, errors.New(RosettaTypes.PrintStruct(bal.Errors))
		}

		balance, err := decodeHexData(bal.Data.Block.Call.Data)
		if err != nil {
			return nil, fmt.Errorf(
				"err encountered for currency %s, token address %s; %v",
				curr.Symbol,
				contractAddress,
				err,
			)
		}

		balances = append(balances, &RosettaTypes.Amount{
			Value:    balance.String(),
			Currency: curr,
			Metadata: map[string]interface{}{
				"status":  bal.Data.Block.Call.Status,
				"gasUsed": bal.Data.Block.Call.GasUsed,
			},
		})
	}

	if len(currencies) == 0 {
		balances = append(balances, nativeBalance)
	}

	return &RosettaTypes.AccountBalanceResponse{
		Balances: balances,
		BlockIdentifier: &RosettaTypes.BlockIdentifier{
			Hash:  bal.Data.Block.Hash,
			Index: bal.Data.Block.Number,
		},
		Metadata: map[string]interface{}{
			"nonce": nonce.Int64(),
			"code":  bal.Data.Block.Account.Code,
		},
	}, nil
}

// graphqlCallResponse is to be used for arbitrary call responses (e.g. ERC20 balance calls)
type graphqlCallResponse struct {
	Errors []struct {
		Message string   `json:"message"`
		Path    []string `json:"path"`
	} `json:"errors"`
	Data struct {
		Block struct {
			Hash   string `json:"hash"`
			Number int64  `json:"number"`
			Call   struct {
				Data    string `json:"data"`
				Status  int64  `json:"status"`
				GasUsed int64  `json:"gasUsed"`
			} `json:"call"`
		} `json:"block"`
	} `json:"data"`
}

// GetTransactionReceiptInput is the input to the call
// method "eth_getTransactionReceipt".
type GetTransactionReceiptInput struct {
	TxHash string `json:"tx_hash"`
}

// Call handles calls to the /call endpoint.
func (ec *Client) Call(
	ctx context.Context,
	request *RosettaTypes.CallRequest,
) (*RosettaTypes.CallResponse, error) {
	switch request.Method { // nolint:gocritic
	case "eth_getTransactionReceipt":
		var input GetTransactionReceiptInput
		if err := RosettaTypes.UnmarshalMap(request.Parameters, &input); err != nil {
			return nil, fmt.Errorf("%w: %s", ErrCallParametersInvalid, err.Error())
		}

		if len(input.TxHash) == 0 {
			return nil, fmt.Errorf("%w:tx_hash missing from params", ErrCallParametersInvalid)
		}

		receipt, err := ec.transactionReceipt(ctx, common.HexToHash(input.TxHash))
		if err != nil {
			return nil, err
		}

		// We cannot use RosettaTypes.MarshalMap because geth uses a custom
		// marshaler to convert *types.Receipt to JSON.
		jsonOutput, err := receipt.MarshalJSON()
		if err != nil {
			return nil, fmt.Errorf("%w: %s", ErrCallOutputMarshal, err.Error())
		}

		var receiptMap map[string]interface{}
		if err := json.Unmarshal(jsonOutput, &receiptMap); err != nil {
			return nil, fmt.Errorf("%w: %s", ErrCallOutputMarshal, err.Error())
		}

		// We must encode data over the wire so we can unmarshal correctly
		return &RosettaTypes.CallResponse{
			Result: receiptMap,
		}, nil
	}

	return nil, fmt.Errorf("%w: %s", ErrCallMethodInvalid, request.Method)
}

func toCallArg(msg ethereum.CallMsg) interface{} {
	arg := map[string]interface{}{
		"from": msg.From,
		"to":   msg.To,
	}
	if len(msg.Data) > 0 {
		arg["data"] = hexutil.Bytes(msg.Data)
	}
	if msg.Value != nil {
		arg["value"] = (*hexutil.Big)(msg.Value)
	}
	if msg.Gas != 0 {
		arg["gas"] = hexutil.Uint64(msg.Gas)
	}
	if msg.GasPrice != nil {
		arg["gasPrice"] = (*hexutil.Big)(msg.GasPrice)
	}
	return arg
}

// CalculateBurntContract implementation is taken from:
// https://github.com/maticnetwork/bor/blob/c227a072418626dd758ceabffd2ea7dadac6eecb/params/config.go#L527
//
// TODO: Depend on maticnetwork fork of go-ethereum instead of stock geth so we don't need to
// copy/paste this.
func (ec *Client) CalculateBurntContract(blockNum uint64) string {
	keys := make([]string, 0, len(ec.burntContract))
	for k := range ec.burntContract {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for i := 0; i < len(keys)-1; i++ {
		valUint, _ := strconv.ParseUint(keys[i], 10, 64)
		valUintNext, _ := strconv.ParseUint(keys[i+1], 10, 64)
		if blockNum > valUint && blockNum < valUintNext {
			return ec.burntContract[keys[i]]
		}
	}
	return ec.burntContract[keys[len(keys)-1]]
}
