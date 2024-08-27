// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {IIBCHandler} from "ibc-solidity/contracts/core/25-handler/IIBCHandler.sol";
import {IBCChannelUpgradableUUPSApp} from "./IBCChannelUpgradableUUPSApp.sol";

contract AppV1 is IBCChannelUpgradableUUPSApp {
    string public v1_state;

    /// @custom:oz-upgrades-unsafe-allow constructor
    constructor(IIBCHandler ibcHandler_) IBCChannelUpgradableUUPSApp(ibcHandler_) { }

    function appv1_init(string calldata v) public {
        initialize();
        v1_state = v;
    }
}

contract AppV1_2 is AppV1 {
    uint256 public v1_2_state;

    /// @custom:oz-upgrades-unsafe-allow constructor
    constructor(IIBCHandler ibcHandler_) AppV1(ibcHandler_) { }

    function appv1_2_upgrade(string calldata v1, uint256 v1_2) public { //onlyOnce, admin
        v1_state = v1;
        v1_2_state = v1_2;
    }
}

contract AppV2 is AppV1 {
    string public  v2_state; // conflict with AppV1_2

    /// @custom:oz-upgrades-unsafe-allow constructor
    constructor(IIBCHandler ibcHandler_) AppV1(ibcHandler_) { }

    function appv2_upgrade(string calldata v1, string calldata v2) public { //onlyOnce, admin
        v1_state = v1;
        v2_state = v2;
    }
}

