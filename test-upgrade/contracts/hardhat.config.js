require("@nomicfoundation/hardhat-toolbox");
require("@openzeppelin/hardhat-upgrades");

/** @type import('hardhat/config').HardhatUserConfig */
const config = {
  solidity: {
    compilers: [ {
      version: "0.8.23",
      settings: {
        viaIR: false,
        optimizer: {
          enabled: true,
          runs: 1000000,
        }
      },
    }, {
      version: "0.8.12"
    } ]
  },
  networks: {
    hardhat: {},
  }
};

if (process.env.DEPLOY_NETWORK != null) {
  const cfg = {
    url: process.env.DEPLOY_RPC_URL,
  };
  if (process.env.DEPLOY_MNEMONIC != null) {
      cfg.accounts = { mnemonic: process.env.DEPLOY_MNEMONIC };
  }
  config.networks[process.env.DEPLOY_NETWORK] = cfg;
}

module.exports = config;

