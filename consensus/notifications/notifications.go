// Copyright (c) 2013-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package notifications

import (
	"fmt"
	"github.com/kaspanet/kaspad/util"
	"github.com/kaspanet/kaspad/util/daghash"
	"sync"
)

// NotificationType represents the type of a notification message.
type NotificationType int

// NotificationCallback is used for a caller to provide a callback for
// notifications about various blockDAG events.
type NotificationCallback func(*Notification)

type NotificationManager struct {
	notifications []NotificationCallback
	sync.RWMutex
}

// Constants for the type of a notification message.
const (
	// NTBlockAdded indicates the associated block was added into
	// the blockDAG.
	NTBlockAdded NotificationType = iota

	// NTChainChanged indicates that selected parent
	// chain had changed.
	NTChainChanged
)

// notificationTypeStrings is a map of notification types back to their constant
// names for pretty printing.
var notificationTypeStrings = map[NotificationType]string{
	NTBlockAdded:   "NTBlockAdded",
	NTChainChanged: "NTChainChanged",
}

func NewManager() *NotificationManager {
	return &NotificationManager{}
}

// String returns the NotificationType in human-readable form.
func (n NotificationType) String() string {
	if s, ok := notificationTypeStrings[n]; ok {
		return s
	}
	return fmt.Sprintf("Unknown Notification Type (%d)", int(n))
}

// Notification defines notification that is sent to the caller via the callback
// function provided during the call to NewManager and consists of a notification type
// as well as associated data that depends on the type as follows:
// 	- Added:     *util.Block
type Notification struct {
	Type NotificationType
	Data interface{}
}

// Subscribe to block DAG notifications. Registers a callback to be executed
// when various events take place. See the documentation on Notification and
// NotificationType for details on the types and contents of notifications.
func (cn *NotificationManager) Subscribe(callback NotificationCallback) {
	cn.Lock()
	defer cn.Unlock()
	cn.notifications = append(cn.notifications, callback)
}

// SendNotification sends a notification with the passed type and data if the
// caller requested notifications by providing a callback function in the call
// to NewManager.
func (cn *NotificationManager) SendNotification(typ NotificationType, data interface{}) {
	// Generate and send the notification.
	n := Notification{Type: typ, Data: data}
	cn.RLock()
	defer cn.RUnlock()
	for _, callback := range cn.notifications {
		callback(&n)
	}
}

// BlockAddedNotificationData defines data to be sent along with a BlockAdded
// notification
type BlockAddedNotificationData struct {
	Block         *util.Block
	WasUnorphaned bool
}

// ChainChangedNotificationData defines data to be sent along with a ChainChanged
// notification
type ChainChangedNotificationData struct {
	RemovedChainBlockHashes []*daghash.Hash
	AddedChainBlockHashes   []*daghash.Hash
}
