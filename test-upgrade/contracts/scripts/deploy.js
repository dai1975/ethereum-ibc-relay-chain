const lib = require('./lib');

var CHAIN_ID = 0n; // set in async main

async function deployIBC(deployer) {
  const logicNames = [
    "IBCClient",
    "IBCConnectionSelfStateNoValidation",
    "IBCChannelHandshake",
    "IBCChannelPacketSendRecv",
    "IBCChannelPacketTimeout",
    "IBCChannelUpgradeInitTryAck",
    "IBCChannelUpgradeConfirmOpenTimeoutCancel",
  ];
  const logics = [];
  for (const name of logicNames) {
    const logic = await lib.deploy(deployer, name);
    logics.push(logic);
  }
  return lib.deploy(deployer, "OwnableIBCHandler", logics.map(l => l.target));
}

async function deployApp(deployer, ibcHandler) {
//  const txOverrides = { unsafeAllow: ["constructor"] };
  const unsafeAllow = [
      "constructor", // IBCChannelUpgradableMockApp, IBCMockApp, Ownable
      "state-variable-immutable", // ibcHandler
      "state-variable-assignment", //closeChannelAllowed
  ];
  const proxyV1 = await lib.deployProxy(deployer, "AppV1", [ibcHandler.target], unsafeAllow, "appv1_init", ["v1"]);
  lib.saveAddress(CHAIN_ID, "AppV1", proxyV1);
  console.log("v1_state:", await proxyV1.v1_state());

  const implV1_2 = await lib.prepareImplementation(deployer, proxyV1, "AppV1_2", [ibcHandler.target], unsafeAllow);
  lib.saveAddress(CHAIN_ID, "AppV1_2__impl", implV1_2);

  const implV2 = await lib.prepareImplementation(deployer, proxyV1, "AppV2", [ibcHandler.target], unsafeAllow);
  lib.saveAddress(CHAIN_ID, "AppV2__impl", implV2);

  await lib.receipt(proxyV1.proposeAppVersion("mockapp-2", {
      implementation: implV2.target,
      initialCalldata: implV2.interface.encodeFunctionData("appv2_upgrade", ["v1_v2", "v2"]),
      consumed: false,
  }));

  return { proxyV1, implV1_2, implV2 };
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
  CHAIN_ID = await hre.ethers.provider.getNetwork().then(n => n.chainId);

  // ethers is available in the global scope
  const [deployer] = await hre.ethers.getSigners();
  const deployerAddress = await deployer.getAddress();
  console.log(
    `Deploying the contracts: chainId=${CHAIN_ID}, account: ${deployerAddress}`
  );
  console.log("Account balance:", (await hre.ethers.provider.getBalance(deployer.getAddress())).toString());

  const ibcHandler = await deployIBC(deployer);
  lib.saveAddress(CHAIN_ID, "IBCHandler", ibcHandler);

  const mockClient = await lib.deploy(deployer, "MockClient", [ibcHandler.target]);
  lib.saveAddress(CHAIN_ID, "MockClient", mockClient);

  const multicall3 = await lib.deploy(deployer, "Multicall3", []);
  lib.saveAddress(CHAIN_ID, "Multicall3", multicall3);

  const app = await deployApp(deployer, ibcHandler);

  await ibcHandler.bindPort("mockapp", app.proxyV1.target);
  await ibcHandler.registerClient("mock-client", mockClient.target);
}

if (require.main === module) {
  main()
    .then(() => process.exit(0))
    .catch((error) => {
      console.error(error);
      process.exit(1);
    });
}
