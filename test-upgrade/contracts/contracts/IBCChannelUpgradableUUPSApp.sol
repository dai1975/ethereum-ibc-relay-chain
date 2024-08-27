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
    // -- variables --------------------------------------------------------
    // override Ownable contract
    address _overrideOwner;

    // NOTE: A module should set an initial appVersion struct in contract constructor or initializer
    mapping(string appVersion => AppInfo) internal appInfos;

    // -- functions --------------------------------------------------------
    /// @custom:oz-upgrades-unsafe-allow constructor
    // note that ibcHandler_ is immutable
    constructor(IIBCHandler ibcHandler_) IBCChannelUpgradableMockApp(ibcHandler_) {}

    function initialize() public initializer {
        // __Ownable_init(msg.sender);
        __UUPSUpgradeable_init();
        _overrideOwner = msg.sender;
    }

    function owner() public view virtual override returns (address) {
        return _overrideOwner;
    }

    // ----------------------------------------------------------
    // UUPSUpgraeable
    function _authorizeUpgrade(address) internal virtual override {
        require(msg.sender == owner());
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

        (Channel.Data memory channel, ) = IIBCHandler(ibcHandler).getChannel(portId, channelId);
        _prepareUpgrade(channel.version, version);
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

        (Channel.Data memory channel, ) = IIBCHandler(ibcHandler).getChannel(portId, channelId);
        _prepareUpgrade(channel.version, version);
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

        (Channel.Data memory channel, ) = IIBCHandler(ibcHandler).getChannel(portId, channelId);
        _upgradeApp(channel.version);
    }

    // ----------------------------------------------------------
    // IBCChannelUpgradableMockApp
    function getAppInfoProposal(string memory version) public view virtual override returns (AppInfo memory) {
        return appInfos[version];
    }
    function proposeAppVersion(string memory version, AppInfo calldata appInfo_) public virtual override onlyOwner {
        require(appInfo_.implementation != address(0), "zero address");
        require(!appInfo_.consumed, "should not be consumed");

        AppInfo storage appInfo = appInfos[version];
        require(appInfo.implementation == address(0), "already set");

        appInfos[version] = appInfo_;
    }

    function _prepareUpgrade(string memory version, string memory newVersion) internal view {
        if (! _compareString(version, newVersion)) {
            AppInfo storage appInfo = appInfos[newVersion];
            require(appInfo.implementation != address(0), "upgrade implementation is zero");
        }
    }

    function _upgradeApp(string memory newVersion) internal {
        AppInfo storage appInfo = appInfos[newVersion];

        if (appInfo.implementation != address(0) && !appInfo.consumed) {
            appInfo.consumed = true;
            // if there is no implementation update, nothing happens here
            if (ERC1967Utils.getImplementation() != appInfo.implementation) {
                ERC1967Utils.upgradeToAndCall(appInfo.implementation, appInfo.initialCalldata); //TODO: check authz
            }
            delete appInfo.initialCalldata;
        }
    }

    function _compareString(string memory a, string memory b) private pure returns (bool) {
        if (bytes(a).length != bytes(b).length) {
            return false;
        }
        return keccak256(abi.encodePacked(a)) == keccak256(abi.encodePacked(b));
    }
}
