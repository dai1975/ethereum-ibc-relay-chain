// TODO: locate ethereum-ibc-relay-chain/contract/
// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

interface IIBCChannelUpgradableAppModuleErrors {
    // ------------------- Errors ------------------- //
}

interface IIBCChannelUpgradableAppModule {
    // ------------------- Data Structures ------------------- //

    /**
     * @dev Proposed AppInfo data
     * @param fields implemantation new implementation address
     * @param initialCalldata calldata just after contract upgraded
     */
    struct AppInfo {
        address implementation;
        bytes initialCalldata;
        bool consumed;
    }

    // ------------------- Functions ------------------- //

    /**
     * @dev Returns the proposed AppInfo for the given version
     */
    function getAppInfoProposal(string memory version) external returns (AppInfo memory);

    /**
     * @dev Propose an Appinfo for the given port and channel
     * @notice This function is only callable by an authorized upgrader
     * The upgrader must call this function before calling `channelUpgradeInit` or `channelUpgradeTry` of the IBC handler
     */
    function proposeAppVersion(string memory version, AppInfo calldata appInfo) external;

}

