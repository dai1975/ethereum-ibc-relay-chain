package ethereum

import (
	"context"
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	gethcrypto "github.com/ethereum/go-ethereum/crypto"
)

func (chain *Chain) CallOpts(ctx context.Context, height int64) *bind.CallOpts {
	opts := &bind.CallOpts{
		From:    gethcrypto.PubkeyToAddress(chain.relayerPrvKey.PublicKey),
		Context: ctx,
	}
	if height > 0 {
		opts.BlockNumber = big.NewInt(height)
	}
	return opts
}

func (chain *Chain) TxOpts(ctx context.Context) *bind.TransactOpts {
	signer := gethtypes.LatestSignerForChainID(chain.chainID)
	prv := chain.relayerPrvKey
	addr := gethcrypto.PubkeyToAddress(prv.PublicKey)
	return &bind.TransactOpts{
		From:     addr,
		GasLimit: 6382056,
		Signer: func(address common.Address, tx *gethtypes.Transaction) (*gethtypes.Transaction, error) {
			logger := chain.GetChainLogger()
			if address != addr {
				err := errors.New("not authorized to sign this account")
				logger.Error("address not match", err, "address", address, "expected", addr)
				return nil, err
			}
			signature, err := gethcrypto.Sign(signer.Hash(tx).Bytes(), prv)
			if err != nil {
				logger.Error("failed to sign tx", err)
				return nil, err
			}
			return tx.WithSignature(signer, signature)
		},
	}
}
