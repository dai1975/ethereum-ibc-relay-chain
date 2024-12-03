package ethereum

import (
	"context"
	"os"
	"fmt"
	"log"
	"testing"
	"math/big"
	"crypto/ecdsa"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/ethclient/simulated"
	"github.com/ethereum/go-ethereum/node"
	"github.com/ethereum/go-ethereum/eth/ethconfig"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
)

type chain struct {
	backend *simulated.Backend
	chainID *big.Int
	privateKey *ecdsa.PrivateKey
}
func newChain(chainID *big.Int) chain {
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		log.Fatal(err)
	}

	auth := bind.NewKeyedTransactor(privateKey)

	balance := new(big.Int)
	balance.SetString("10000000000000000000", 10) // 10 eth in wei

	address := auth.From
	genesisAlloc := map[common.Address]core.GenesisAccount{
		address: {
			Balance: balance,
		},
	}

	backend := simulated.NewBackend(
		genesisAlloc,
		func(codeConf *node.Config, ethConf *ethconfig.Config) {
			ethConf.Genesis.Config.ChainID = new(big.Int).Set(chainID) //clone
		},
	)
	return chain{
		backend: backend,
		chainID: chainID,
		privateKey: privateKey,
	}
}

var setup struct {
	chainA, chainB chain
}

func TestMain(m *testing.M) {
	chainA := newChain(big.NewInt(1009))
	defer chainA.backend.Close()

	chainB := newChain(big.NewInt(2003))
	defer chainA.backend.Close()

	setup.chainA = chainA
	setup.chainA = chainB

	r := m.Run()
	os.Exit(r)
}

func TestSetup(t *testing.T) {
	log.Printf("chainA: %d\n", setup.chainA.chainID)
	log.Printf("chainB: %d\n", setup.chainB.chainID)
}

func TestSend(t *testing.T) {
	client := setup.chainA.backend.Client()

	auth := bind.NewKeyedTransactor(setup.chainA.privateKey)
	fromAddress := auth.From

	nonce, err := client.PendingNonceAt(context.Background(), fromAddress)
	if err != nil {
		log.Fatal(err)
	}

	value := big.NewInt(1000000000000000000) // in wei (1 eth)
	head, err := client.HeaderByNumber(context.Background(), nil)
	if err != nil {
		log.Fatal(err)
	}
	gasPrice := new(big.Int).Add(head.BaseFee, big.NewInt(params.GWei))

	toAddress := common.HexToAddress("0x4592d8f8d7b001e72cb26a73e4fa1806a51ac79d")
	var data []byte
	tx := types.NewTx(&types.DynamicFeeTx{
		ChainID: setup.chainA.chainID,
		Nonce: nonce,
		To: &toAddress,
		Value: value,
		GasTipCap: big.NewInt(params.GWei),
		GasFeeCap: gasPrice,
		Gas: 1000000,
		Data: data,
	});

	signer := types.NewLondonSigner(setup.chainA.chainID);
	signedTx, err := types.SignTx(tx, signer, setup.chainA.privateKey);
	if err != nil {
		log.Fatal(err)
	}

	err = client.SendTransaction(context.Background(), signedTx)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("tx sent: %s\n", signedTx.Hash().Hex())

	setup.chainA.backend.Commit()

	receipt, err := client.TransactionReceipt(context.Background(), signedTx.Hash())
	if err != nil {
		log.Fatal(err)
	}
	if receipt == nil {
		log.Fatal("receipt is nil. Forgot to commit?")
	}

	if receipt.Status != 1 {
		log.Fatal("receipt failed.")
	}
}

/*
func main() {
	privateKey, err := crypto.GenerateKey()
	if err != nil {
	log.Fatal(err)
	}

	auth := bind.NewKeyedTransactor(privateKey)

	balance := new(big.Int)
	balance.SetString("10000000000000000000", 10) // 10 eth in wei

	address := auth.From
	genesisAlloc := map[common.Address]core.GenesisAccount{
		address: {
			Balance: balance,
		},
	}

	backend := simulated.NewBackend(
		genesisAlloc,
		func(codeConf *node.Config, ethConf *ethconfig.Config) {
			ethConf.Genesis.Config.ChainID = big.NewInt(3939)
		},
	)
	defer backend.Close()

	client := backend.Client()

	chainID, err := client.ChainID(context.Background());
	if err != nil {
		log.Fatal(err);
	}
	log.Printf("chainId: %d", chainID);
	signer := types.NewLondonSigner(chainID);

	fromAddress := auth.From
	nonce, err := client.PendingNonceAt(context.Background(), fromAddress)
	if err != nil {
		log.Fatal(err)
	}

	value := big.NewInt(1000000000000000000) // in wei (1 eth)
	head, err := client.HeaderByNumber(context.Background(), nil)
	if err != nil {
		log.Fatal(err)
	}
	gasPrice := new(big.Int).Add(head.BaseFee, big.NewInt(params.GWei))

	toAddress := common.HexToAddress("0x4592d8f8d7b001e72cb26a73e4fa1806a51ac79d")
	var data []byte
	tx := types.NewTx(&types.DynamicFeeTx{
		ChainID: chainID,
		Nonce: nonce,
		To: &toAddress,
		Value: value,
		GasTipCap: big.NewInt(params.GWei),
		GasFeeCap: gasPrice,
		Gas: 1000000,
		Data: data,
	});
	signedTx, err := types.SignTx(tx, signer, privateKey);
	if err != nil {
		log.Fatal(err)
	}

	err = client.SendTransaction(context.Background(), signedTx)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("tx sent: %s\n", signedTx.Hash().Hex())

	backend.Commit()

	receipt, err := client.TransactionReceipt(context.Background(), signedTx.Hash())
	if err != nil {
		log.Fatal(err)
	}
	if receipt == nil {
		log.Fatal("receipt is nil. Forgot to commit?")
	}

	fmt.Printf("status: %v\n", receipt.Status) // status: 1
}
*/
