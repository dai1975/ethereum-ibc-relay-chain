const lib = require('./lib');

async function testV2(address) {
  const proxyV2 = await hre.ethers.getContractAt("AppV2", address);
  console.log("v1_state: ", await proxyV2.v1_state());
  console.log("v2_state: ", await proxyV2.v2_state());
}

async function main() {
  // This is just a convenience check
  if (network.name === "hardhat") {
    console.warn(
      "You are trying to deploy a contract to the Hardhat Network, which" +
        "gets automatically created and destroyed every time. Use the Hardhat" +
        " option '--network localhost'"
    );
  }

  if (process.env.V2_ADDRESS != null) {
      await testV2(process.env.V2_ADDRESS);
  }
}

if (require.main === module) {
  main()
    .then(() => process.exit(0))
    .catch((error) => {
      console.error(error);
      process.exit(1);
    });
}
