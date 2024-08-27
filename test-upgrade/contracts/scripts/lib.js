const receipt = async (ptxr /* Promise<ethers.TransactionResponse> */) /* : Promise<ethers.TransactionReceipt> */ => {
  return ptxr.then(txr => {
    return txr.wait(3);
  }).then(r => {
    if (r == null) {
      return Promise.reject("wait returns null");
    } else {
      return Promise.resolve(r);
    }
  });
}

const saveAddress = (chainId, contractName, contract) => {
  const fs = require("fs");
  const path = require("path");

  const dirpath = path.join("addresses", chainId.toString());
  if (!fs.existsSync(dirpath)) {
    fs.mkdirSync(dirpath, {recursive: true});
  }

  const filepath = path.join(dirpath, contractName);
  fs.writeFileSync(filepath, contract.target);

  console.log(`${contractName} address:`, contract.target);
}

const deploy = async (deployer, contractName, args = []) => {
  const factory = await hre.ethers.getContractFactory(contractName);
  const contract = await factory.connect(deployer).deploy(...args);
  await contract.waitForDeployment();
  return contract;
}

const deployProxy = async (deployer, contractName, constructorArgs, unsafeAllow, initializer, initialArgs) => {
  const factory = await hre.ethers.getContractFactory(contractName).then(f => f.connect(deployer));
  const proxyOptions /* : DeployProxyOptions */ = {
    txOverrides: {},
    unsafeAllow: unsafeAllow ?? [],
    constructorArgs,
    initializer: initializer ?? false,
    redeployImplementation: 'always'
  };
  const proxyContract = await upgrades.deployProxy(
    factory,
    initialArgs ?? [],
    proxyOptions
  );
  await proxyContract.waitForDeployment();
  return proxyContract.connect(deployer);
}

const prepareImplementation = async(deployer, proxy, contractName, constructorArgs, unsafeAllow) => {
    const factory = await hre.ethers.getContractFactory(contractName).then(f => f.connect(deployer));
    const implOptions /* : DeployImplementationOptions */ = {
        constructorArgs,
        txOverrides: {},
        unsafeAllow: unsafeAllow ?? [],
        redeployImplementation: 'always',
        getTxResponse: true
    };
    const implReceipt = await receipt(hre.upgrades.prepareUpgrade(proxy, factory, implOptions));
    const implContract = await hre.ethers.getContractAt(contractName, implReceipt.contractAddress);
    return implContract.connect(deployer);
}

module.exports = {
  receipt, saveAddress, deploy, deployProxy, prepareImplementation,
}
