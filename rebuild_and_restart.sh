#!/bin/bash

set -e

echo "========================================"
echo "Rebuilding and Restarting Cosmos EVM Node"
echo "========================================"

# Stop any running nodes
echo ""
echo "1. Stopping any running nodes..."
pkill evmd || true
sleep 2

# Clean build directory
echo ""
echo "2. Cleaning build directory..."
rm -rf build/

# Rebuild the binary
echo ""
echo "3. Building evmd binary..."
export PATH=/opt/homebrew/bin:$PATH
mkdir -p build
cd evmd && CGO_ENABLED=1 /opt/homebrew/bin/go build -mod=readonly -tags "netgo" -ldflags '-X github.com/cosmos/cosmos-sdk/version.Name=os -X github.com/cosmos/cosmos-sdk/version.AppName=evmd -w -s' -o ../build/evmd ./cmd/evmd

if [ $? -ne 0 ]; then
    echo "❌ Build failed!"
    cd ..
    exit 1
fi
cd ..

echo "✅ Build successful!"

# Clean data directory for fresh start
echo ""
echo "4. Cleaning data directory..."
rm -rf ~/.evmd/

# Start the node
echo ""
echo "5. Starting node with local_node.sh..."
echo "   (This will initialize a fresh chain with the new Keeper)"
echo ""

# Add build directory to PATH so local_node.sh can find evmd
export PATH="$(pwd)/build:$PATH"
./local_node.sh
