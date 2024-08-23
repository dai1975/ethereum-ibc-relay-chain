// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import "@openzeppelin/contracts-upgradeable/proxy/utils/UUPSUpgradeable.sol";
//import "@openzeppelin/contracts-upgradeable/access/OwnableUpgradeable.sol";
import "@openzeppelin/contracts/proxy/ERC1967/ERC1967Utils.sol";

// TODO: locate this interface file on ethereum-ibc-relay-chain/contract
import {IIBCChannelUpgradableAppModule, IIBCChannelUpgradableAppModuleErrors} from "./IIBCChannelUpgradableAppModule.sol";

import {IBCChannelUpgradableMockApp} from "ibc-solidity/contracts/apps/mock/IBCChannelUpgradableMockApp.sol";
import {IBCChannelUpgradableModuleBase} from "ibc-solidity/contracts/apps/commons/IBCChannelUpgradableModule.sol";
import {IBCAppBase} from "ibc-solidity/contracts/apps/commons/IBCAppBase.sol";
import {Channel, UpgradeFields, Timeout} from "ibc-solidity/contracts/proto/Channel.sol";

import {IIBCHandler} from "ibc-solidity/contracts/core/25-handler/IIBCHandler.sol";

contract IBCChannelUpgradableUUPSApp is
  //OwnableUpgradeable, // IBCMockApp is already Ownable
  UUPSUpgradeable,
  IBCChannelUpgradableMockApp,
  IIBCChannelUpgradableAppModule,
  IIBCChannelUpgradableAppModuleErrors
{
    AppInfo internal appInfoCandidate;
    AppInfo internal appInfo;
    bool internal appInfoApplied; //default false

    constructor(IIBCHandler ibcHandler_) IBCChannelUpgradableMockApp(ibcHandler_) {}

    // ----------------------------------------------------------
    // ownable upgradeable
    function initialize() public initializer {
        // __Ownable_init(msg.sender);
        __UUPSUpgradeable_init();
    }

    function _authorizeUpgrade(address) internal virtual override {
        require(msg.sender == owner() ||
                msg.sender == address(this)); //called by this._upgradeApp()
    }

    // ----------------------------------------------------------
    // IBCChannelUpgradable
    /**
     * @dev See {IERC165-supportsInterface}.
     */
    function supportsInterface(bytes4 interfaceId)
        public
        view
        virtual
        override(IBCChannelUpgradableMockApp)
        returns (bool)
    {
        return super.supportsInterface(interfaceId)
            || interfaceId == type(IIBCChannelUpgradableAppModule).interfaceId;

    }

    /**
     * @dev See {IIBCModuleUpgrade-onChanUpgradeInit}
     */
    function onChanUpgradeInit(
        string calldata portId,
        string calldata channelId,
        uint64 upgradeSequence,
        UpgradeFields.Data calldata proposedUpgradeFields
    ) public view virtual override onlyIBC returns (string memory version) {
        version = super.onChanUpgradeInit(portId, channelId, upgradeSequence, proposedUpgradeFields);
        // ERROR: view function
        //appInfo = appInfoCandidate;
        //appInfoApplied = false;
    }

    /**
     * @dev See {IIBCModuleUpgrade-onChanUpgradeTry}
     */
    function onChanUpgradeTry(
        string calldata portId,
        string calldata channelId,
        uint64 upgradeSequence,
        UpgradeFields.Data calldata proposedUpgradeFields
    ) public view virtual override onlyIBC returns (string memory version) {
        version = super.onChanUpgradeTry(portId, channelId, upgradeSequence, proposedUpgradeFields);
        // ERROR: view function
        //appInfo = appInfoCandidate;
        //appInfoApplied = false;
    }

    /**
     * @dev See {IIBCModuleUpgrade-onChanUpgradeAck}
     */
    function onChanUpgradeAck(
        string calldata portId,
        string calldata channelId,
        uint64 upgradeSequence,
        string calldata counterpartyVersion
    ) public view virtual override onlyIBC {
        super.onChanUpgradeAck(portId, channelId, upgradeSequence, counterpartyVersion);
    }

    /**
     * @dev See {IIBCModuleUpgrade-onChanUpgradeOpen}
     */
    function onChanUpgradeOpen(
        string calldata portId,
        string calldata channelId,
        uint64 upgradeSequence
    ) public virtual override onlyIBC {
        super.onChanUpgradeOpen(portId, channelId, upgradeSequence);
        _upgradeApp();
    }

    // ----------------------------------------------------------
    // IBCChannelUpgradableMockApp
    function getAppInfoProposal() public view virtual override returns (AppInfo memory) {
        return appInfo;
    }

    function proposeAppInfo(AppInfo calldata appInfo_) public virtual override onlyOwner {
        appInfoCandidate = appInfo_;
    }

    function removeUpgradeProposal() public virtual override onlyOwner {
        // TODO: delete appInfoCandidate.initialCalldata;
    }

    function _upgradeApp() internal {
        if (appInfoApplied || appInfo.implementation == address(0)) {
            return;
        }

        if (ERC1967Utils.getImplementation() == appInfo.implementation) {
            return;
        }

        ERC1967Utils.upgradeToAndCall(appInfo.implementation, appInfo.initialCalldata);

        appInfoApplied = true;
    }

}
