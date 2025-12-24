package main

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"flag"
	"log"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

// This is a small helper binary to send a single gasless transaction
// against a locally running evmd node.
//
// It assumes that your github.com/cosmos/go-ethereum fork defines a
// custom GaslessTx tx data type and a corresponding GaslessTxType.
// You will need to adjust the TODO section in buildGaslessTx once
// that type exists in the fork.
func main() {
	rpcURL := flag.String("rpc", "http://localhost:8545", "Ethereum JSON-RPC endpoint of evmd")
	toAddr := flag.String("to", "0xADDRESS", "target contract address for gasless tx (must match GaslessAllowedToAddressStr)")
	privHex := flag.String("privkey", "", "hex-encoded private key for the sender (optional: random if empty)")
	gasLimit := flag.Uint64("gas", 200000, "gas limit for the gasless tx")

	flag.Parse()

	if *toAddr == "0xADDRESS" {
		log.Fatalf("--to must be set to the same address as GaslessAllowedToAddressStr in ante/evm/mono_decorator.go")
	}

	client, err := ethclient.Dial(*rpcURL)
	if err != nil {
		log.Fatalf("failed to connect to RPC %s: %v", *rpcURL, err)
	}
	defer client.Close()

	// Load or generate the sender key
	var priv *ecdsa.PrivateKey
	if *privHex == "" {
		priv, err = crypto.GenerateKey()
		if err != nil {
			log.Fatalf("failed to generate private key: %v", err)
		}
		log.Printf("[info] generated random sender key; this account likely has 0 balance (good for gasless test)")
	} else {
		b, err := hex.DecodeString(strip0x(*privHex))
		if err != nil {
			log.Fatalf("invalid --privkey hex: %v", err)
		}
		priv, err = crypto.ToECDSA(b)
		if err != nil {
			log.Fatalf("failed to parse private key: %v", err)
		}
	}

	pubKey := priv.Public().(*ecdsa.PublicKey)
	fromAddr := crypto.PubkeyToAddress(*pubKey)
	log.Printf("[info] from address: %s", fromAddr.Hex())

	ctx := context.Background()

	nonce, err := client.PendingNonceAt(ctx, fromAddr)
	if err != nil {
		log.Fatalf("failed to get nonce: %v", err)
	}

	chainID, err := client.NetworkID(ctx)
	if err != nil {
		log.Fatalf("failed to get network ID: %v", err)
	}
	log.Printf("[info] chain ID: %s", chainID.String())

	to := common.HexToAddress(*toAddr)

	// Build the gasless transaction (fork-specific)
	tx, err := buildGaslessTx(&to, nonce, *gasLimit)
	if err != nil {
		log.Fatalf("failed to build gasless tx: %v", err)
	}

	// Sign the transaction
	// NOTE: this assumes your fork wires GaslessTxType into LatestSignerForChainID
	signer := types.LatestSignerForChainID(chainID)
	signed, err := types.SignTx(tx, signer, priv)
	if err != nil {
		log.Fatalf("failed to sign tx: %v", err)
	}

	// Send the transaction
	if err := client.SendTransaction(ctx, signed); err != nil {
		log.Fatalf("failed to send tx: %v", err)
	}

	log.Printf("[ok] gasless tx sent: %s", signed.Hash().Hex())
	log.Printf("     from: %s", fromAddr.Hex())
	log.Printf("     to:   %s", to.Hex())
}

// buildGaslessTx constructs a gasless transaction using a standard LegacyTx
// with gasPrice=0. The ante handler will detect that this transaction is
// to a whitelisted contract and sponsor the fees.
func buildGaslessTx(to *common.Address, nonce uint64, gasLimit uint64) (*types.Transaction, error) {
	// Create a legacy transaction with zero gas price
	// The ante handler will intercept this and sponsor the gas fees
	tx := types.NewTransaction(
		nonce,
		*to,
		nil,         // value (no ETH transfer)
		gasLimit,
		nil,         // gasPrice = 0 for gasless
		[]byte{},    // empty data
	)

	return tx, nil
}

func strip0x(s string) string {
	if len(s) >= 2 && (s[0:2] == "0x" || s[0:2] == "0X") {
		return s[2:]
	}
	return s
}
