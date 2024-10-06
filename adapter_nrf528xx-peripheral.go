//go:build softdevice && s113v7

package bluetooth

// This file implements the event handler for SoftDevices with only peripheral
// mode support. This includes the S113.

/*
#include "nrf_sdm.h"
#include "nrf_nvic.h"
#include "ble.h"
#include "ble_gap.h"
*/
import "C"

import (
	"unsafe"
)

func handleEvent() {
	id := eventBuf.header.evt_id
	switch {
	case id >= C.BLE_GAP_EVT_BASE && id <= C.BLE_GAP_EVT_LAST:
		gapEvent := eventBuf.evt.unionfield_gap_evt()
		switch id {
		case C.BLE_GAP_EVT_CONNECTED:
			if debug {
				println("evt: connected in peripheral role")
			}
			currentConnection.handle.Reg = uint16(gapEvent.conn_handle)
			connectEvent := gapEvent.params.unionfield_connected()
			device := Device{
				Address:          Address{makeMACAddress(connectEvent.peer_addr)},
				connectionHandle: gapEvent.conn_handle,
			}
			DefaultAdapter.connectHandler(device, true)
		case C.BLE_GAP_EVT_DISCONNECTED:
			if debug {
				println("evt: disconnected")
			}
			currentConnection.handle.Reg = C.BLE_CONN_HANDLE_INVALID
			// Auto-restart advertisement if needed.
			if defaultAdvertisement.isAdvertising.Get() != 0 {
				// The advertisement was running but was automatically stopped
				// by the connection event.
				// Note that it cannot be restarted during connect like this,
				// because it would need to be reconfigured as a non-connectable
				// advertisement. That's left as a future addition, if
				// necessary.
				C.sd_ble_gap_adv_start(defaultAdvertisement.handle, connCfgTag)
			}
			device := Device{
				connectionHandle: gapEvent.conn_handle,
			}
			DefaultAdapter.connectHandler(device, false)
		case C.BLE_GAP_EVT_DATA_LENGTH_UPDATE_REQUEST:
			// We need to respond with sd_ble_gap_data_length_update. Setting
			// both parameters to nil will make sure we send the default values.
			C.sd_ble_gap_data_length_update(gapEvent.conn_handle, nil, nil)
		case C.BLE_GAP_EVT_DATA_LENGTH_UPDATE:
			// ignore confirmation of data length successfully updated
		case C.BLE_GAP_EVT_PHY_UPDATE_REQUEST:
			phyUpdateRequest := gapEvent.params.unionfield_phy_update_request()
			C.sd_ble_gap_phy_update(gapEvent.conn_handle, &phyUpdateRequest.peer_preferred_phys)
		case C.BLE_GAP_EVT_PHY_UPDATE:
			// ignore confirmation of phy successfully updated
		default:
			if debug {
				println("unknown GAP event:", id)
			}
		}
	case id >= C.BLE_GATTS_EVT_BASE && id <= C.BLE_GATTS_EVT_LAST:
		gattsEvent := eventBuf.evt.unionfield_gatts_evt()
		switch id {
		case C.BLE_GATTS_EVT_WRITE:
			writeEvent := gattsEvent.params.unionfield_write()
			len := writeEvent.len - writeEvent.offset
			data := (*[255]byte)(unsafe.Pointer(&writeEvent.data[0]))[:len:len]
			handler := DefaultAdapter.getCharWriteHandler(writeEvent.handle)
			if handler != nil {
				handler.callback(Connection(gattsEvent.conn_handle), int(writeEvent.offset), data)
			}
		case C.BLE_GATTS_EVT_SYS_ATTR_MISSING:
			// This event is generated when reading the Generic Attribute
			// service. It appears to be necessary for bonded devices.
			// From the docs:
			// > If the pointer is NULL, the system attribute info is
			// > initialized, assuming that the application does not have any
			// > previously saved system attribute data for this device.
			// Maybe we should look at the error, but as there's not really a
			// way to handle it, ignore it.
			C.sd_ble_gatts_sys_attr_set(gattsEvent.conn_handle, nil, 0, 0)
		case C.BLE_GATTS_EVT_EXCHANGE_MTU_REQUEST:
			rsp := gattsEvent.params.unionfield_exchange_mtu_request()
			effectiveMtu := min(DefaultAdapter.cfg.Gatt.AttMtu, uint16(rsp.client_rx_mtu))
			if debug {
				println("mtu exchange requested. self:", DefaultAdapter.cfg.Gatt.AttMtu, ", peer:", rsp.client_rx_mtu, ", effective:", effectiveMtu)
			}

			var errCode = C.sd_ble_gatts_exchange_mtu_reply(gattsEvent.conn_handle, C.uint16_t(effectiveMtu))
			if debug {
				println("mtu exchange replied, err:", Error(errCode).Error())
			}
		case C.BLE_GATTS_EVT_HVN_TX_COMPLETE:
			// ignore confirmation of a notification successfully sent
		default:
			if debug {
				println("unknown GATTS event:", id, id-C.BLE_GATTS_EVT_BASE)
			}
		}
	default:
		if debug {
			println("unknown event:", id)
		}
	}
}

func min(a, b uint16) uint16 {
	if a < b {
		return a
	}
	return b
}
