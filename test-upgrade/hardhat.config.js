require("@nomicfoundation/hardhat-toolbox");

/** @type import('hardhat/config').HardhatUserConfig */
module.exports = {
  solidity: {
    version: "0.8.24",
    settings: {
      viaIR: false,
      optimizer: {
        enabled: true,
        runs: 1000000,
      },
    }
  }
};
