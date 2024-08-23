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
    }

    // ------------------- Functions ------------------- //

    /**
     * @dev Returns the proposed AppInfo for the given port and channel
     */
    function getAppInfoProposal() external view returns (AppInfo memory);

    /**
     * @dev Propose an Appinfo for the given port and channel
     * @notice This function is only callable by an authorized upgrader
     * The upgrader must call this function before calling `channelUpgradeInit` or `channelUpgradeTry` of the IBC handler
     */
    function proposeAppInfo(AppInfo calldata appInfo) external;

    /**
     * @dev Removes the proposed AppInfo for the given port and channel
     * @notice This function is only callable by an authorized upgrader
     */
    function removeUpgradeProposal() external;

}

