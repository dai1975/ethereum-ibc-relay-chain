package ethereum

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	math "math"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/gogoproto/proto"
	clienttypes "github.com/cosmos/ibc-go/v8/modules/core/02-client/types"
	conntypes "github.com/cosmos/ibc-go/v8/modules/core/03-connection/types"
	chantypes "github.com/cosmos/ibc-go/v8/modules/core/04-channel/types"
	"github.com/cosmos/ibc-go/v8/modules/core/exported"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/hyperledger-labs/yui-relayer/core"
	"github.com/hyperledger-labs/yui-relayer/log"

	"github.com/datachainlab/ethereum-ibc-relay-chain/pkg/client"
	"github.com/datachainlab/ethereum-ibc-relay-chain/pkg/contract/ibchandler"
	"github.com/datachainlab/ethereum-ibc-relay-chain/pkg/contract/multicall3"
)

// SendMsgs sends msgs to the chain
func (c *Chain) SendMsgs(msgs []sdk.Msg) ([]core.MsgID, error) {
	ctx := context.TODO()
	// if src's connection is OPEN, dst's connection is OPEN or TRYOPEN, so we can skip to update client commitments
	skipUpdateClientCommitment, err := c.confirmConnectionOpened(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to confirm connection opened: %w", err)
	}
	logger := c.GetChainLogger()
	ethereumSignerLogger := c.ethereumSigner.GetLogger()
	defer c.ethereumSigner.SetLogger(ethereumSignerLogger)

	var msgIDs []core.MsgID

	iter := NewCallIter(msgs, skipUpdateClientCommitment)
	for !iter.End() {
		from := iter.Cursor()
		logger := &log.RelayLogger{Logger: logger.With(logAttrMsgIndexFrom, from)}
		c.ethereumSigner.SetLogger(logger);

		tx, err := iter.SendTx(ctx, c)
		logger.Logger = logger.With(logAttrMsgIndexTo, iter.Cursor() -1)

		if err != nil {
			logger.Error("failed to send msg", err)
			return nil, err
		} else if tx == nil {
			break
		} else {
			logger.Logger = logger.With(logAttrTxHash, tx.Hash())
		}

		if rawTxData, err := tx.MarshalBinary(); err != nil {
			logger.Error("failed to encode tx", err)
		} else {
			logger.Logger = logger.With(logAttrRawTxData, hex.EncodeToString(rawTxData))
		}

		receipt, err := c.client.WaitForReceiptAndGet(ctx, tx.Hash())
		if err != nil {
			logger.Error("failed to get receipt", err)
			return nil, err
		} else {
			logger.Logger = logger.With(
				logAttrBlockHash, receipt.BlockHash,
				logAttrBlockNumber, receipt.BlockNumber.Uint64(),
				logAttrTxIndex, receipt.TransactionIndex,
			)
		}

		if receipt.Status == gethtypes.ReceiptStatusFailed {
			if revertReason, rawErrorData, err := c.getRevertReasonFromReceipt(ctx, receipt); err != nil {
				// Raw error data may be available even if revert reason isn't available.
				logger.Logger = logger.With(logAttrRawErrorData, hex.EncodeToString(rawErrorData))
				logger.Error("failed to get revert reason", err)
			} else {
				logger.Logger = logger.With(
					logAttrRawErrorData, hex.EncodeToString(rawErrorData),
					logAttrRevertReason, revertReason,
				)
			}

			err := errors.New("tx execution reverted")
			logger.Error("tx execution reverted", err)
			return nil, err
		}
		logger.Info("successfully sent tx")
		if c.msgEventListener != nil {
			for i := from; i < iter.Cursor(); i++ {
				if err := c.msgEventListener.OnSentMsg([]sdk.Msg{msgs[i]}); err != nil {
					logger.Error("failed to OnSendMsg call", err, "index", i)
				}
			}
		}
		for i := from; i < iter.Cursor(); i++ {
			msgIDs = append(msgIDs, NewMsgID(tx.Hash()))
		}
	}
	return msgIDs, nil
}

func (c *Chain) GetMsgResult(id core.MsgID) (core.MsgResult, error) {
	logger := c.GetChainLogger()

	msgID, ok := id.(*MsgID)
	if !ok {
		return nil, fmt.Errorf("unexpected message id type: %T", id)
	}
	ctx := context.TODO()
	txHash := msgID.TxHash()
	receipt, err := c.client.WaitForReceiptAndGet(ctx, txHash)
	if err != nil {
		return nil, err
	}
	if receipt.Status == gethtypes.ReceiptStatusSuccessful {
		return c.makeMsgResultFromReceipt(&receipt.Receipt, "")
	}
	revertReason, rawErrorData, err := c.getRevertReasonFromReceipt(ctx, receipt)
	if err != nil {
		logger.Error("failed to get revert reason", err,
			logAttrRawErrorData, hex.EncodeToString(rawErrorData),
			logAttrTxHash, msgID.TxHashHex,
			logAttrBlockHash, receipt.BlockHash.Hex(),
			logAttrBlockNumber, receipt.BlockNumber.Uint64(),
			logAttrTxIndex, receipt.TransactionIndex,
		)
	}
	return c.makeMsgResultFromReceipt(&receipt.Receipt, revertReason)
}

func (c *Chain) TxCreateClient(opts *bind.TransactOpts, msg *clienttypes.MsgCreateClient) (*gethtypes.Transaction, error) {
	var clientState exported.ClientState
	if err := c.codec.UnpackAny(msg.ClientState, &clientState); err != nil {
		return nil, err
	}
	clientStateBytes, err := proto.Marshal(msg.ClientState)
	if err != nil {
		return nil, err
	}
	consensusStateBytes, err := proto.Marshal(msg.ConsensusState)
	if err != nil {
		return nil, err
	}
	return c.ibcHandler.CreateClient(opts, ibchandler.IIBCClientMsgCreateClient{
		ClientType:          clientState.ClientType(),
		ProtoClientState:    clientStateBytes,
		ProtoConsensusState: consensusStateBytes,
	})
}

func (c *Chain) TxUpdateClient(opts *bind.TransactOpts, msg *clienttypes.MsgUpdateClient, skipUpdateClientCommitment bool) (*gethtypes.Transaction, error) {
	clientMessageBytes, err := proto.Marshal(msg.ClientMessage)
	if err != nil {
		return nil, err
	}
	m := ibchandler.IIBCClientMsgUpdateClient{
		ClientId:           msg.ClientId,
		ProtoClientMessage: clientMessageBytes,
	}
	// if `skipUpdateClientCommitment` is true and `allowLCFunctions` is not nil,
	// the relayer calls `routeUpdateClient` to constructs an UpdateClient tx to the LC contract.
	// ref. https://github.com/hyperledger-labs/yui-ibc-solidity/blob/main/docs/adr/adr-001.md
	if skipUpdateClientCommitment && c.allowLCFunctions != nil {
		lcAddr, fnSel, args, err := c.ibcHandler.RouteUpdateClient(c.CallOpts(opts.Context, 0), m)
		if err != nil {
			return nil, fmt.Errorf("failed to route update client: %w", err)
		}
		// ensure that the contract and function are allowed
		if c.allowLCFunctions.IsAllowed(lcAddr, fnSel) {
			log.GetLogger().Info("contract function is allowed", "contract", lcAddr.Hex(), "selector", fmt.Sprintf("0x%x", fnSel))
			calldata := append(fnSel[:], args...)
			return bind.NewBoundContract(lcAddr, abi.ABI{}, c.client, c.client, c.client).RawTransact(opts, calldata)
		}
		// fallback to send an UpdateClient to the IBC handler contract
		log.GetLogger().Warn("contract function is not allowed", "contract", lcAddr.Hex(), "selector", fmt.Sprintf("0x%x", fnSel))
	}
	return c.ibcHandler.UpdateClient(opts, m)
}

func (c *Chain) TxConnectionOpenInit(opts *bind.TransactOpts, msg *conntypes.MsgConnectionOpenInit) (*gethtypes.Transaction, error) {
	return c.ibcHandler.ConnectionOpenInit(opts, ibchandler.IIBCConnectionMsgConnectionOpenInit{
		ClientId: msg.ClientId,
		Counterparty: ibchandler.CounterpartyData{
			ClientId:     msg.Counterparty.ClientId,
			ConnectionId: msg.Counterparty.ConnectionId,
			Prefix:       ibchandler.MerklePrefixData(msg.Counterparty.Prefix),
		},
		DelayPeriod: msg.DelayPeriod,
	})
}

func (c *Chain) TxConnectionOpenTry(opts *bind.TransactOpts, msg *conntypes.MsgConnectionOpenTry) (*gethtypes.Transaction, error) {
	clientStateBytes, err := proto.Marshal(msg.ClientState)
	if err != nil {
		return nil, err
	}
	var versions []ibchandler.VersionData
	for _, v := range msg.CounterpartyVersions {
		versions = append(versions, ibchandler.VersionData(*v))
	}
	return c.ibcHandler.ConnectionOpenTry(opts, ibchandler.IIBCConnectionMsgConnectionOpenTry{
		Counterparty: ibchandler.CounterpartyData{
			ClientId:     msg.Counterparty.ClientId,
			ConnectionId: msg.Counterparty.ConnectionId,
			Prefix:       ibchandler.MerklePrefixData(msg.Counterparty.Prefix),
		},
		DelayPeriod:             msg.DelayPeriod,
		ClientId:                msg.ClientId,
		ClientStateBytes:        clientStateBytes,
		CounterpartyVersions:    versions,
		ProofInit:               msg.ProofInit,
		ProofClient:             msg.ProofClient,
		ProofConsensus:          msg.ProofConsensus,
		ProofHeight:             pbToHandlerHeight(msg.ProofHeight),
		ConsensusHeight:         pbToHandlerHeight(msg.ConsensusHeight),
		HostConsensusStateProof: msg.HostConsensusStateProof,
	})
}

func (c *Chain) TxConnectionOpenAck(opts *bind.TransactOpts, msg *conntypes.MsgConnectionOpenAck) (*gethtypes.Transaction, error) {
	clientStateBytes, err := proto.Marshal(msg.ClientState)
	if err != nil {
		return nil, err
	}
	return c.ibcHandler.ConnectionOpenAck(opts, ibchandler.IIBCConnectionMsgConnectionOpenAck{
		ConnectionId:     msg.ConnectionId,
		ClientStateBytes: clientStateBytes,
		Version: ibchandler.VersionData{
			Identifier: msg.Version.Identifier,
			Features:   msg.Version.Features,
		},
		CounterpartyConnectionId: msg.CounterpartyConnectionId,
		ProofTry:                 msg.ProofTry,
		ProofClient:              msg.ProofClient,
		ProofConsensus:           msg.ProofConsensus,
		ProofHeight:              pbToHandlerHeight(msg.ProofHeight),
		ConsensusHeight:          pbToHandlerHeight(msg.ConsensusHeight),
		HostConsensusStateProof:  msg.HostConsensusStateProof,
	})
}

func (c *Chain) TxConnectionOpenConfirm(opts *bind.TransactOpts, msg *conntypes.MsgConnectionOpenConfirm) (*gethtypes.Transaction, error) {
	return c.ibcHandler.ConnectionOpenConfirm(opts, ibchandler.IIBCConnectionMsgConnectionOpenConfirm{
		ConnectionId: msg.ConnectionId,
		ProofAck:     msg.ProofAck,
		ProofHeight:  pbToHandlerHeight(msg.ProofHeight),
	})
}

func (c *Chain) TxChannelOpenInit(opts *bind.TransactOpts, msg *chantypes.MsgChannelOpenInit) (*gethtypes.Transaction, error) {
	return c.ibcHandler.ChannelOpenInit(opts, ibchandler.IIBCChannelHandshakeMsgChannelOpenInit{
		PortId: msg.PortId,
		Channel: ibchandler.ChannelData{
			State:          uint8(msg.Channel.State),
			Ordering:       uint8(msg.Channel.Ordering),
			Counterparty:   ibchandler.ChannelCounterpartyData(msg.Channel.Counterparty),
			ConnectionHops: msg.Channel.ConnectionHops,
			Version:        msg.Channel.Version,
		},
	})
}

func (c *Chain) TxChannelOpenTry(opts *bind.TransactOpts, msg *chantypes.MsgChannelOpenTry) (*gethtypes.Transaction, error) {
	return c.ibcHandler.ChannelOpenTry(opts, ibchandler.IIBCChannelHandshakeMsgChannelOpenTry{
		PortId: msg.PortId,
		Channel: ibchandler.ChannelData{
			State:          uint8(msg.Channel.State),
			Ordering:       uint8(msg.Channel.Ordering),
			Counterparty:   ibchandler.ChannelCounterpartyData(msg.Channel.Counterparty),
			ConnectionHops: msg.Channel.ConnectionHops,
			Version:        msg.Channel.Version,
		},
		CounterpartyVersion: msg.CounterpartyVersion,
		ProofInit:           msg.ProofInit,
		ProofHeight:         pbToHandlerHeight(msg.ProofHeight),
	})
}

func (c *Chain) TxChannelOpenAck(opts *bind.TransactOpts, msg *chantypes.MsgChannelOpenAck) (*gethtypes.Transaction, error) {
	return c.ibcHandler.ChannelOpenAck(opts, ibchandler.IIBCChannelHandshakeMsgChannelOpenAck{
		PortId:                msg.PortId,
		ChannelId:             msg.ChannelId,
		CounterpartyVersion:   msg.CounterpartyVersion,
		CounterpartyChannelId: msg.CounterpartyChannelId,
		ProofTry:              msg.ProofTry,
		ProofHeight:           pbToHandlerHeight(msg.ProofHeight),
	})
}

func (c *Chain) TxChannelOpenConfirm(opts *bind.TransactOpts, msg *chantypes.MsgChannelOpenConfirm) (*gethtypes.Transaction, error) {
	return c.ibcHandler.ChannelOpenConfirm(opts, ibchandler.IIBCChannelHandshakeMsgChannelOpenConfirm{
		PortId:      msg.PortId,
		ChannelId:   msg.ChannelId,
		ProofAck:    msg.ProofAck,
		ProofHeight: pbToHandlerHeight(msg.ProofHeight),
	})
}

func (c *Chain) TxRecvPacket(opts *bind.TransactOpts, msg *chantypes.MsgRecvPacket) (*gethtypes.Transaction, error) {
	return c.ibcHandler.RecvPacket(opts, ibchandler.IIBCChannelRecvPacketMsgPacketRecv{
		Packet: ibchandler.Packet{
			Sequence:           msg.Packet.Sequence,
			SourcePort:         msg.Packet.SourcePort,
			SourceChannel:      msg.Packet.SourceChannel,
			DestinationPort:    msg.Packet.DestinationPort,
			DestinationChannel: msg.Packet.DestinationChannel,
			Data:               msg.Packet.Data,
			TimeoutHeight:      ibchandler.HeightData(msg.Packet.TimeoutHeight),
			TimeoutTimestamp:   msg.Packet.TimeoutTimestamp,
		},
		Proof:       msg.ProofCommitment,
		ProofHeight: pbToHandlerHeight(msg.ProofHeight),
	})
}

func (c *Chain) TxAcknowledgement(opts *bind.TransactOpts, msg *chantypes.MsgAcknowledgement) (*gethtypes.Transaction, error) {
	return c.ibcHandler.AcknowledgePacket(opts, ibchandler.IIBCChannelAcknowledgePacketMsgPacketAcknowledgement{
		Packet: ibchandler.Packet{
			Sequence:           msg.Packet.Sequence,
			SourcePort:         msg.Packet.SourcePort,
			SourceChannel:      msg.Packet.SourceChannel,
			DestinationPort:    msg.Packet.DestinationPort,
			DestinationChannel: msg.Packet.DestinationChannel,
			Data:               msg.Packet.Data,
			TimeoutHeight:      ibchandler.HeightData(msg.Packet.TimeoutHeight),
			TimeoutTimestamp:   msg.Packet.TimeoutTimestamp,
		},
		Acknowledgement: msg.Acknowledgement,
		Proof:           msg.ProofAcked,
		ProofHeight:     pbToHandlerHeight(msg.ProofHeight),
	})
}

func (c *Chain) SendTx(opts *bind.TransactOpts, msg sdk.Msg, skipUpdateClientCommitment bool) (*gethtypes.Transaction, error) {
	logger := c.GetChainLogger()
	var (
		tx  *gethtypes.Transaction
		err error
	)
	switch msg := msg.(type) {
	case *clienttypes.MsgCreateClient:
		tx, err = c.TxCreateClient(opts, msg)
	case *clienttypes.MsgUpdateClient:
		tx, err = c.TxUpdateClient(opts, msg, skipUpdateClientCommitment)
	case *conntypes.MsgConnectionOpenInit:
		tx, err = c.TxConnectionOpenInit(opts, msg)
	case *conntypes.MsgConnectionOpenTry:
		tx, err = c.TxConnectionOpenTry(opts, msg)
	case *conntypes.MsgConnectionOpenAck:
		tx, err = c.TxConnectionOpenAck(opts, msg)
	case *conntypes.MsgConnectionOpenConfirm:
		tx, err = c.TxConnectionOpenConfirm(opts, msg)
	case *chantypes.MsgChannelOpenInit:
		tx, err = c.TxChannelOpenInit(opts, msg)
	case *chantypes.MsgChannelOpenTry:
		tx, err = c.TxChannelOpenTry(opts, msg)
	case *chantypes.MsgChannelOpenAck:
		tx, err = c.TxChannelOpenAck(opts, msg)
	case *chantypes.MsgChannelOpenConfirm:
		tx, err = c.TxChannelOpenConfirm(opts, msg)
	case *chantypes.MsgRecvPacket:
		tx, err = c.TxRecvPacket(opts, msg)
	case *chantypes.MsgAcknowledgement:
		tx, err = c.TxAcknowledgement(opts, msg)
	// case *transfertypes.MsgTransfer:
	// 	err = c.client.transfer(msg)
	default:
		logger.Error("failed to send msg", errors.New("illegal msg type"), "msg", msg)
		panic("illegal msg type")
	}
	return tx, err
}

func (c *Chain) getRevertReasonFromReceipt(ctx context.Context, receipt *client.Receipt) (string, []byte, error) {
	var errorData []byte
	if receipt.HasRevertReason() {
		errorData = receipt.RevertReason
	} else if c.config.EnableDebugTrace {
		callFrame, err := c.client.DebugTraceTransaction(ctx, receipt.TxHash)
		if err != nil {
			return "", nil, err
		} else if len(callFrame.Output) == 0 {
			return "", nil, fmt.Errorf("execution reverted without error data")
		}
		errorData = callFrame.Output
	} else {
		return "", nil, fmt.Errorf("no way to get revert reason")
	}

	revertReason, err := c.errorRepository.ParseError(errorData)
	if err != nil {
		return "", errorData, fmt.Errorf("failed to parse error: %v", err)
	}
	return revertReason, errorData, nil
}

func (c *Chain) getRevertReasonFromRpcError(err error) (string, []byte, error) {
	if de, ok := err.(rpc.DataError); !ok {
		return "", nil, fmt.Errorf("failed with unexpected error type: errorType=%T", err)
	} else if de.ErrorData() == nil {
		return "", nil, fmt.Errorf("failed without error data")
	} else if errorData, ok := de.ErrorData().(string); !ok {
		return "", nil, fmt.Errorf("failed with unexpected error data type: errorDataType=%T", de.ErrorData())
	} else {
		errorData := common.FromHex(errorData)
		revertReason, err := c.errorRepository.ParseError(errorData)
		if err != nil {
			return "", errorData, fmt.Errorf("failed to parse error: %v", err)
		}
		return revertReason, errorData, nil
	}
}

func (c *Chain) parseRpcError(err error) (string, string) {
	revertReason, rawErrorData, err := c.getRevertReasonFromRpcError(err)
	if err != nil {
		revertReason = fmt.Sprintf("failed to get revert reason: %s", err.Error())
	}
	// Note that Raw error data may be available even if revert reason isn't available.
	return revertReason, hex.EncodeToString(rawErrorData)
}


func estimateGas(
	ctx context.Context,
	c *Chain,
	tx *gethtypes.Transaction,
	doRound bool, // return rounded gas limit when gas limit is over
	base_logger *log.RelayLogger,
) (uint64, error) {
	logger := &log.RelayLogger{ Logger: base_logger.Logger }

	if rawTxData, err := tx.MarshalBinary(); err != nil {
		logger.Error("failed to encode tx", err)
	} else {
		logger.Logger = logger.With(logAttrRawTxData, hex.EncodeToString(rawTxData))
	}

	estimatedGas, err := c.client.EstimateGasFromTx(ctx, tx)
	if err != nil {
		if revertReason, rawErrorData, err := c.getRevertReasonFromRpcError(err); err != nil {
			// Raw error data may be available even if revert reason isn't available.
			logger.Logger = logger.With(logAttrRawErrorData, hex.EncodeToString(rawErrorData))
			logger.Error("failed to get revert reason", err)
		} else {
			logger.Logger = logger.With(
				logAttrRawErrorData, hex.EncodeToString(rawErrorData),
				logAttrRevertReason, revertReason,
			)
		}

		logger.Error("failed to estimate gas", err)
		return 0, err
	}

	txGasLimit := estimatedGas * c.Config().GasEstimateRate.Numerator / c.Config().GasEstimateRate.Denominator
	if txGasLimit > c.Config().MaxGasLimit {
		if !doRound {
			return 0, fmt.Errorf("estimated gas exceeds max gas limit")
		}

		logger.Warn("estimated gas exceeds max gas limit",
			logAttrEstimatedGas, txGasLimit,
			logAttrMaxGasLimit, c.Config().MaxGasLimit,
		)
		return c.Config().MaxGasLimit, nil
	}

	return txGasLimit, nil
}


type CallIter struct {
	msgs []sdk.Msg
	cursor int
	skipUpdateClientCommitment bool
	// for multicall
	txs []gethtypes.Transaction
	msgTypeNames []string
}
func NewCallIter(msgs []sdk.Msg, skipUpdateClientCommitment bool) CallIter {
	return CallIter {
		msgs: msgs,
		cursor: 0,
		skipUpdateClientCommitment: skipUpdateClientCommitment,
	}
}
func (iter *CallIter) Cursor() int {
	return iter.cursor
}
func (iter *CallIter) Current() sdk.Msg {
	return iter.msgs[iter.cursor]
}
func (iter *CallIter) End() bool {
	return len(iter.msgs) <= iter.cursor
}
func (iter *CallIter) Next() {
	if !iter.End() {
		iter.cursor += 1
	}
}

func (iter *CallIter) SendTx(ctx context.Context, c *Chain) (*gethtypes.Transaction, error) {
	if c.multicall3 == nil {
		return iter.sendSingleTx(ctx, c)
	} else {
		return iter.sendMultiTx(ctx, c)
	}
}

func (iter *CallIter) sendSingleTx(ctx context.Context, c *Chain) (*gethtypes.Transaction, error) {
	if iter.End() {
		return nil, nil
	}

	logger := c.GetChainLogger()
	logger = &log.RelayLogger{Logger: logger.With(
		logAttrMsgIndexFrom, iter.Cursor(),
		logAttrMsgType, fmt.Sprintf("%T", iter.Current()),
	)}

	opts, err := c.TxOpts(ctx, true);
	if err != nil {
		return nil, err
	}

	// gas estimation
	{
		opts.GasLimit = math.MaxUint64
		opts.NoSend = true
		tx, err := c.SendTx(opts, iter.Current(), iter.skipUpdateClientCommitment)
		if err != nil {
			logger.Error("failed to build tx for gas estimation", err)
			return nil, err
		}

		txGasLimit, err := estimateGas(ctx, c, tx, true, logger)
		if err != nil {
			return nil, err
		}
		opts.GasLimit = txGasLimit
	}

	opts.NoSend = false
	tx, err := c.SendTx(opts, iter.Current(), iter.skipUpdateClientCommitment)
	if err != nil {
		logger.Error("failed to send msg", err)
		return nil, err
	}
	iter.Next()
	return tx, nil
}

func (iter *CallIter) sendMultiTx(ctx context.Context, c *Chain) (*gethtypes.Transaction, error) {
	if (iter.End()) {
		return nil, nil
	}
	// now iter.cursor < len(iter.msgs)

	logger := c.GetChainLogger()

	opts, err := c.TxOpts(ctx, true);
	if err != nil {
		if err != nil {
			return nil, err
		}
	}

	if iter.txs == nil { // create txs at first multicall call
		opts.GasLimit = math.MaxUint64
		opts.NoSend = true
		txs := make([]gethtypes.Transaction, 0, len(iter.msgs))
		msgTypeNames := make([]string, 0, len(iter.msgs))

		for i := 0; i < len(iter.msgs); i++ {
			msgTypeName := fmt.Sprintf("%T", iter.msgs[i])
			msgTypeNames = append(msgTypeNames, msgTypeName)

			tx, err := c.SendTx(opts, iter.msgs[i], iter.skipUpdateClientCommitment)
			if err != nil {
				logger := &log.RelayLogger{Logger: logger.With(
					logAttrMsgIndexFrom, i,
					logAttrMsgIndexTo, i,
					logAttrMsgType, msgTypeName,
				)}
				logger.Error("failed to build tx for gas estimation", err)
				return nil, err
			}
			if tx.To() == nil {
				err2 := fmt.Errorf("no target address")
				logger.Error("failed to construct Multicall3Call", err2)
				return nil, err2
			}
			txs = append(txs, *tx)
		}
		iter.txs = txs
		iter.msgTypeNames = msgTypeNames
	}

	var (
		lastOkCalls []multicall3.Multicall3Call = nil
		lastOkGasLimit uint64 = 0
	)
	count, err := findItems(
		len(iter.msgs) - iter.Cursor(),
		func(count int) (error) {
			from := iter.Cursor()
			to := from + count

			logger := &log.RelayLogger{Logger: logger.With(
				logAttrMsgIndexFrom, from,
				logAttrMsgIndexTo, from + count - 1,
				logAttrMsgType, strings.Join(iter.msgTypeNames[0 : from + count], ","),
			)}

			calls := make([]multicall3.Multicall3Call, 0, count)
			for i := from; i < to; i++ {
				calls = append(calls, multicall3.Multicall3Call{
					Target: *iter.txs[i].To(),
					CallData: iter.txs[i].Data(),
				})
			}

			opts.GasLimit = math.MaxUint64
			opts.NoSend = true
			multiTx, err := c.multicall3.Aggregate(opts, calls)
			if err != nil {
				return err
			}

			txGasLimit, err := estimateGas(ctx, c, multiTx, 1 == count, logger)
			if err != nil {
				return err
			}

			lastOkGasLimit = txGasLimit
			lastOkCalls = calls
			return nil
		})

	logger = &log.RelayLogger{Logger: logger.With(
		logAttrMsgIndexFrom, iter.Cursor(),
		logAttrMsgIndexTo, iter.Cursor() + count - 1,
		logAttrMsgType, strings.Join(iter.msgTypeNames[0 : iter.Cursor() + count], ","),
	)}

	if err != nil {
		logger.Error("failed to prepare multicall tx", err)
		return nil, err
	}

	opts.GasLimit = lastOkGasLimit
	opts.NoSend = false
	tx, err := c.multicall3.Aggregate(opts, lastOkCalls)
	if err != nil {
		logger.Error("failed to send multicall tx", err)
		return nil, err
	}
	logger.Info("succeeded in sending multicall")
	iter.cursor += count
	return tx, nil
}

func findItems(
	size int,
	f func(int) (error),
) (int, error) {
	if (size <= 0) {
		return 0, fmt.Errorf("empty items")
	}

	// hot path
	if err := f(size); err == nil {
		return size, nil
	}
	// binary search
	if i := sort.Search(size-1, func(n int) bool { return f(n+1) != nil }); i == 0 {
		return 0, fmt.Errorf("not found")
	} else {
		return i, nil
	}
}

