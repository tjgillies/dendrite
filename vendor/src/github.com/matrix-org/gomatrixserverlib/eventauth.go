/* Copyright 2016-2017 Vector Creations Ltd
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package gomatrixserverlib

import (
	"encoding/json"
	"fmt"
	"sort"
)

const (
	join   = "join"
	ban    = "ban"
	leave  = "leave"
	invite = "invite"
	public = "public"
)

// StateNeeded lists the event types and state_keys needed to authenticate an event.
type StateNeeded struct {
	// Is the m.room.create event needed to auth the event.
	Create bool
	// Is the m.room.join_rules event needed to auth the event.
	JoinRules bool
	// Is the m.room.power_levels event needed to auth the event.
	PowerLevels bool
	// List of m.room.member state_keys needed to auth the event
	Member []string
	// List of m.room.third_party_invite state_keys
	ThirdPartyInvite []string
}

// StateNeededForAuth returns the event types and state_keys needed to authenticate an event.
// This takes a list of events to facilitate bulk processing when doing auth checks as part of state conflict resolution.
func StateNeededForAuth(events []Event) (result StateNeeded) {
	var members []string
	var thirdpartyinvites []string

	for _, event := range events {
		switch event.Type() {
		case "m.room.create":
			// The create event doesn't require any state to authenticate.
			// https://github.com/matrix-org/synapse/blob/v0.18.5/synapse/api/auth.py#L123
		case "m.room.aliases":
			// Alias events need:
			//  * The create event.
			//    https://github.com/matrix-org/synapse/blob/v0.18.5/synapse/api/auth.py#L128
			// Alias events need no further authentication.
			// https://github.com/matrix-org/synapse/blob/v0.18.5/synapse/api/auth.py#L160
			result.Create = true
		case "m.room.member":
			// Member events need:
			//  * The previous membership of the target.
			//    https://github.com/matrix-org/synapse/blob/v0.18.5/synapse/api/auth.py#L355
			//  * The current membership state of the sender.
			//    https://github.com/matrix-org/synapse/blob/v0.18.5/synapse/api/auth.py#L348
			//  * The join rules for the room if the event is a join event.
			//    https://github.com/matrix-org/synapse/blob/v0.18.5/synapse/api/auth.py#L361
			//  * The power levels for the room.
			//    https://github.com/matrix-org/synapse/blob/v0.18.5/synapse/api/auth.py#L370
			//  * And optionally may require a m.third_party_invite event
			//    https://github.com/matrix-org/synapse/blob/v0.18.5/synapse/api/auth.py#L393
			content, err := newMemberContentFromEvent(event)
			if err != nil {
				// If we hit an error decoding the content we ignore it here.
				// The event will be rejected when the actual checks encounter the same error.
				continue
			}
			result.Create = true
			result.PowerLevels = true
			stateKey := event.StateKey()
			if stateKey != nil {
				members = append(members, event.Sender(), *stateKey)
			}
			if content.Membership == join {
				result.JoinRules = true
			}
			if content.ThirdPartyInvite != nil {
				token, err := thirdPartyInviteToken(content.ThirdPartyInvite)
				if err != nil {
					// If we hit an error decoding the content we ignore it here.
					// The event will be rejected when the actual checks encounter the same error.
					continue
				} else {
					thirdpartyinvites = append(thirdpartyinvites, token)
				}
			}

		default:
			// All other events need:
			//  * The membership of the sender.
			//    https://github.com/matrix-org/synapse/blob/v0.18.5/synapse/api/auth.py#L177
			//  * The power levels for the room.
			//    https://github.com/matrix-org/synapse/blob/v0.18.5/synapse/api/auth.py#L196
			result.Create = true
			result.PowerLevels = true
			members = append(members, event.Sender())
		}
	}

	// Deduplicate the state keys.
	sort.Strings(members)
	result.Member = members[:unique(sort.StringSlice(members))]
	sort.Strings(thirdpartyinvites)
	result.ThirdPartyInvite = thirdpartyinvites[:unique(sort.StringSlice(thirdpartyinvites))]
	return
}

// Remove duplicate items from a sorted list.
// Takes the same interface as sort.Sort
// Returns the length of the data without duplicates
// Uses the last occurrence of a duplicate.
// O(n).
func unique(data sort.Interface) int {
	length := data.Len()
	if length == 0 {
		return 0
	}
	j := 0
	for i := 1; i < length; i++ {
		if data.Less(i-1, i) {
			data.Swap(i-1, j)
			j++
		}
	}
	data.Swap(length-1, j)
	return j + 1
}

// thirdPartyInviteToken extracts the token from the third_party_invite.
func thirdPartyInviteToken(thirdPartyInviteData json.RawMessage) (string, error) {
	var thirdPartyInvite struct {
		Signed struct {
			Token string `json:"token"`
		} `json:"signed"`
	}
	if err := json.Unmarshal(thirdPartyInviteData, &thirdPartyInvite); err != nil {
		return "", err
	}
	if thirdPartyInvite.Signed.Token == "" {
		return "", fmt.Errorf("missing 'third_party_invite.signed.token' JSON key")
	}
	return thirdPartyInvite.Signed.Token, nil
}

// AuthEvents are the state events needed to authenticate an event.
type AuthEvents interface {
	// Create returns the m.room.create event for the room.
	Create() (*Event, error)
	// JoinRules returns the m.room.join_rules event for the room.
	JoinRules() (*Event, error)
	// PowerLevels returns the m.room.power_levels event for the room.
	PowerLevels() (*Event, error)
	// Member returns the m.room.member event for the given user_id state_key.
	Member(stateKey string) (*Event, error)
	// ThirdPartyInvite returns the m.room.third_party_invite event for the
	// given state_key
	ThirdPartyInvite(stateKey string) (*Event, error)
}

// A NotAllowed error is returned if an event does not pass the auth checks.
type NotAllowed struct {
	Message string
}

func (a *NotAllowed) Error() string {
	return "eventauth: " + a.Message
}

func errorf(message string, args ...interface{}) error {
	return &NotAllowed{Message: fmt.Sprintf(message, args...)}
}

// Allowed checks whether an event is allowed by the auth events.
// It returns a NotAllowed error if the event is not allowed.
// If there was an error loading the auth events then it returns that error.
func Allowed(event Event, authEvents AuthEvents) error {
	switch event.Type() {
	case "m.room.create":
		return createEventAllowed(event)
	case "m.room.aliases":
		return aliasEventAllowed(event, authEvents)
	case "m.room.member":
		return memberEventAllowed(event, authEvents)
	case "m.room.power_levels":
		return powerLevelsEventAllowed(event, authEvents)
	case "m.room.redaction":
		return redactEventAllowed(event, authEvents)
	default:
		return defaultEventAllowed(event, authEvents)
	}
}

// createEventAllowed checks whether the m.room.create event is allowed.
// It returns an error if the event is not allowed.
func createEventAllowed(event Event) error {
	if !event.StateKeyEquals("") {
		return errorf("create event state key is not empty: %v", event.StateKey())
	}
	roomIDDomain, err := domainFromID(event.RoomID())
	if err != nil {
		return err
	}
	senderDomain, err := domainFromID(event.Sender())
	if err != nil {
		return err
	}
	if senderDomain != roomIDDomain {
		return errorf("create event room ID domain does not match sender: %q != %q", roomIDDomain, senderDomain)
	}
	if len(event.PrevEvents()) > 0 {
		return errorf("create event must be the first event in the room: found %d prev_events", len(event.PrevEvents()))
	}
	return nil
}

// memberEventAllowed checks whether the m.room.member event is allowed.
// Membership events have different authentication rules to ordinary events.
func memberEventAllowed(event Event, authEvents AuthEvents) error {
	allower, err := newMembershipAllower(authEvents, event)
	if err != nil {
		return err
	}
	return allower.membershipAllowed(event)
}

// aliasEventAllowed checks whether the m.room.aliases event is allowed.
// Alias events have different authentication rules to ordinary events.
func aliasEventAllowed(event Event, authEvents AuthEvents) error {
	// The alias events have different auth rules to ordinary events.
	// In particular we allow any server to send a m.room.aliases event without checking if the sender is in the room.
	// This allows server admins to update the m.room.aliases event for their server when they change the aliases on their server.
	// https://github.com/matrix-org/synapse/blob/v0.18.5/synapse/api/auth.py#L143-L160

	create, err := newCreateContentFromAuthEvents(authEvents)

	senderDomain, err := domainFromID(event.Sender())
	if err != nil {
		return err
	}

	if event.RoomID() != create.roomID {
		return errorf("create event has different roomID: %q != %q", event.RoomID(), create.roomID)
	}

	// Check that server is allowed in the room by the m.room.federate flag.
	if err := create.domainAllowed(senderDomain); err != nil {
		return err
	}

	// Check that event is a state event.
	// Check that the state key matches the server sending this event.
	// https://github.com/matrix-org/synapse/blob/v0.18.5/synapse/api/auth.py#L158
	if !event.StateKeyEquals(senderDomain) {
		return errorf("alias state_key does not match sender domain, %q != %q", senderDomain, event.StateKey())
	}

	return nil
}

// powerLevelsEventAllowed checks whether the m.room.power_levels event is allowed.
// It returns an error if the event is not allowed or if there was a problem
// loading the auth events needed.
func powerLevelsEventAllowed(event Event, authEvents AuthEvents) error {
	allower, err := newEventAllower(authEvents, event.Sender())
	if err != nil {
		return err
	}

	// power level events must pass the default checks.
	// These checks will catch if the user has a high enough level to set a m.room.power_levels state event.
	if err = allower.commonChecks(event); err != nil {
		return err
	}

	// Parse the power levels.
	newPowerLevels, err := newPowerLevelContentFromEvent(event)
	if err != nil {
		return err
	}

	// Check that the user levels are all valid user IDs
	// https://github.com/matrix-org/synapse/blob/v0.18.5/synapse/api/auth.py#L1063
	for userID := range newPowerLevels.userLevels {
		if !isValidUserID(userID) {
			return errorf("Not a valid user ID: %q", userID)
		}
	}

	// Grab the old power level event so that we can check if the event existed.
	var oldEvent *Event
	if oldEvent, err = authEvents.PowerLevels(); err != nil {
		return err
	} else if oldEvent == nil {
		// If this is the first power level event then it can set the levels to
		// any value it wants to.
		// https://github.com/matrix-org/synapse/blob/v0.18.5/synapse/api/auth.py#L1074
		return nil
	}

	// Grab the old levels so that we can compare new the levels against them.
	oldPowerLevels := allower.powerLevels
	senderLevel := oldPowerLevels.userLevel(event.Sender())

	// Check that the changes in event levels are allowed.
	if err = checkEventLevels(senderLevel, oldPowerLevels, newPowerLevels); err != nil {
		return err
	}

	// Check that the changes in user levels are allowed.
	return checkUserLevels(senderLevel, event.Sender(), oldPowerLevels, newPowerLevels)
}

// checkEventLevels checks that the changes in event levels are allowed.
func checkEventLevels(senderLevel int64, oldPowerLevels, newPowerLevels powerLevelContent) error {
	type levelPair struct {
		old int64
		new int64
	}
	// Build a list of event levels to check.
	// This differs slightly in behaviour from the code in synapse because it will use the
	// default value if a level is not present in one of the old or new events.

	// First add all the named levels.
	levelChecks := []levelPair{
		{oldPowerLevels.banLevel, newPowerLevels.banLevel},
		{oldPowerLevels.inviteLevel, newPowerLevels.inviteLevel},
		{oldPowerLevels.kickLevel, newPowerLevels.kickLevel},
		{oldPowerLevels.redactLevel, newPowerLevels.redactLevel},
		{oldPowerLevels.stateDefaultLevel, newPowerLevels.stateDefaultLevel},
		{oldPowerLevels.eventDefaultLevel, newPowerLevels.eventDefaultLevel},
	}

	// Then add checks for each event key in the new levels.
	// We use the default values for non-state events when applying the checks.
	// TODO: the per event levels do not distinguish between state and non-state events.
	// However the default values do make that distinction. We may want to change this.
	// For example if there is an entry for "my.custom.type" events it sets the level
	// for sending the event with and without a "state_key". But if there is no entry
	// for "my.custom.type it will use the state default when sent with a "state_key"
	// and will use the event default when sent without.
	const (
		isStateEvent = false
	)
	for eventType := range newPowerLevels.eventLevels {
		levelChecks = append(levelChecks, levelPair{
			oldPowerLevels.eventLevel(eventType, isStateEvent),
			newPowerLevels.eventLevel(eventType, isStateEvent),
		})
	}

	// Then add checks for each event key in the old levels.
	// Some of these will be duplicates of the ones added using the keys from
	// the new levels. But it doesn't hurt to run the checks twice for the same level.
	for eventType := range oldPowerLevels.eventLevels {
		levelChecks = append(levelChecks, levelPair{
			oldPowerLevels.eventLevel(eventType, isStateEvent),
			newPowerLevels.eventLevel(eventType, isStateEvent),
		})
	}

	// Check each of the levels in the list.
	for _, level := range levelChecks {
		// Check if the level is being changed.
		if level.old == level.new {
			// Levels are always allowed to stay the same.
			continue
		}

		// Users are allowed to change the level for an event if:
		//   * the old level was less than or equal to their own
		//   * the new level was less than or equal to their own
		// https://github.com/matrix-org/synapse/blob/v0.18.5/synapse/api/auth.py#L1134

		// Check if the user is trying to set any of the levels to above their own.
		if senderLevel < level.new {
			return errorf(
				"sender with level %d is not allowed to change level from %d to %d"+
					" because the new level is above the level of the sender",
				senderLevel, level.old, level.new,
			)
		}

		// Check if the user is trying to set a level that was above their own.
		if senderLevel < level.old {
			return errorf(
				"sender with level %d is not allowed to change level from %d to %d"+
					" because the current level is above the level of the sender",
				senderLevel, level.old, level.new,
			)
		}
	}

	return nil
}

// checkUserLevels checks that the changes in user levels are allowed.
func checkUserLevels(senderLevel int64, senderID string, oldPowerLevels, newPowerLevels powerLevelContent) error {
	type levelPair struct {
		old    int64
		new    int64
		userID string
	}

	// Build a list of user levels to check.
	// This differs slightly in behaviour from the code in synapse because it will use the
	// default value if a level is not present in one of the old or new events.

	// First add the user default level.
	userLevelChecks := []levelPair{
		{oldPowerLevels.userDefaultLevel, newPowerLevels.userDefaultLevel, ""},
	}

	// Then add checks for each user key in the new levels.
	for userID := range newPowerLevels.userLevels {
		userLevelChecks = append(userLevelChecks, levelPair{
			oldPowerLevels.userLevel(userID), newPowerLevels.userLevel(userID), userID,
		})
	}

	// Then add checks for each user key in the old levels.
	// Some of these will be duplicates of the ones added using the keys from
	// the new levels. But it doesn't hurt to run the checks twice for the same level.
	for userID := range oldPowerLevels.userLevels {
		userLevelChecks = append(userLevelChecks, levelPair{
			oldPowerLevels.userLevel(userID), newPowerLevels.userLevel(userID), userID,
		})
	}

	// Check each of the levels in the list.
	for _, level := range userLevelChecks {
		// Check if the level is being changed.
		if level.old == level.new {
			// Levels are always allowed to stay the same.
			continue
		}

		// Users are allowed to change the level of other users if:
		//   * the old level was less than their own
		//   * the new level was less than or equal to their own
		// They are allowed to change their own level if:
		//   * the new level was less than or equal to their own
		// https://github.com/matrix-org/synapse/blob/v0.18.5/synapse/api/auth.py#L1126-L1127
		// https://github.com/matrix-org/synapse/blob/v0.18.5/synapse/api/auth.py#L1134

		// Check if the user is trying to set any of the levels to above their own.
		if senderLevel < level.new {
			return errorf(
				"sender with level %d is not allowed change user level from %d to %d"+
					" because the new level is above the level of the sender",
				senderLevel, level.old, level.new,
			)
		}

		// Check if the user is changing their own user level.
		if level.userID == senderID {
			// Users are always allowed to reduce their own user level.
			// We know that the user is reducing their level because of the previous checks.
			continue
		}

		// Check if the user is changing the level that was above or the same as their own.
		if senderLevel <= level.old {
			return errorf(
				"sender with level %d is not allowed to change user level from %d to %d"+
					" because the old level is equal to or above the level of the sender",
				senderLevel, level.old, level.new,
			)
		}
	}

	return nil
}

// redactEventAllowed checks whether the m.room.redaction event is allowed.
// It returns an error if the event is not allowed or if there was a problem
// loading the auth events needed.
func redactEventAllowed(event Event, authEvents AuthEvents) error {
	allower, err := newEventAllower(authEvents, event.Sender())
	if err != nil {
		return err
	}

	// redact events must pass the default checks,
	if err = allower.commonChecks(event); err != nil {
		return err
	}

	senderDomain, err := domainFromID(event.Sender())
	if err != nil {
		return err
	}

	redactDomain, err := domainFromID(event.Redacts())
	if err != nil {
		return err
	}

	// Servers are always allowed to redact their own messages.
	// This is so that users can redact their own messages, but since
	// we don't know which user ID sent the message being redacted
	// the only check we can do is to compare the domains of the
	// sender and the redacted event.
	// We leave it up to the sending server to implement the additional checks
	// to ensure that only events that should be redacted are redacted.
	if senderDomain == redactDomain {
		return nil
	}

	// Otherwise the sender must have enough power.
	// This allows room admins and ops to redact messages sent by other servers.
	senderLevel := allower.powerLevels.userLevel(event.Sender())
	redactLevel := allower.powerLevels.redactLevel
	if senderLevel >= redactLevel {
		return nil
	}

	return errorf(
		"%q is not allowed to redact message from %q. %d < %d",
		event.Sender(), redactDomain, senderLevel, redactLevel,
	)
}

// defaultEventAllowed checks whether the event is allowed by the default
// checks for events.
// It returns an error if the event is not allowed or if there was a
// problem loading the auth events needed.
func defaultEventAllowed(event Event, authEvents AuthEvents) error {
	allower, err := newEventAllower(authEvents, event.Sender())
	if err != nil {
		return err
	}

	return allower.commonChecks(event)
}

// An eventAllower has the information needed to authorise all events types
// other than m.room.create, m.room.member and m.room.aliases which are special.
type eventAllower struct {
	// The content of the m.room.create.
	create createContent
	// The content of the m.room.member event for the sender.
	member memberContent
	// The content of the m.room.power_levels event for the room.
	powerLevels powerLevelContent
}

// newEventAllower loads the information needed to authorise an event sent
// by a given user ID from the auth events.
func newEventAllower(authEvents AuthEvents, senderID string) (e eventAllower, err error) {
	if e.create, err = newCreateContentFromAuthEvents(authEvents); err != nil {
		return
	}
	if e.member, err = newMemberContentFromAuthEvents(authEvents, senderID); err != nil {
		return
	}
	if e.powerLevels, err = newPowerLevelContentFromAuthEvents(authEvents, e.create.Creator); err != nil {
		return
	}
	return
}

// commonChecks does the checks that are applied to all events types other than
// m.room.create, m.room.member, or m.room.alias.
func (e *eventAllower) commonChecks(event Event) error {
	if event.RoomID() != e.create.roomID {
		return errorf("create event has different roomID: %q != %q", event.RoomID(), e.create.roomID)
	}

	sender := event.Sender()
	stateKey := event.StateKey()

	if err := e.create.userIDAllowed(sender); err != nil {
		return err
	}

	// Check that the sender is in the room.
	// Every event other than m.room.create, m.room.member and m.room.aliases require this.
	if e.member.Membership != join {
		return errorf("sender %q not in room", sender)
	}

	senderLevel := e.powerLevels.userLevel(sender)
	eventLevel := e.powerLevels.eventLevel(event.Type(), stateKey != nil)
	if senderLevel < eventLevel {
		return errorf(
			"sender %q is not allowed to send event. %d < %d",
			event.Sender(), senderLevel, eventLevel,
		)
	}

	// Check that all state_keys that begin with '@' are only updated by users
	// with that ID.
	if stateKey != nil && len(*stateKey) > 0 && (*stateKey)[0] == '@' {
		if *stateKey != sender {
			return errorf(
				"sender %q is not allowed to modify the state belonging to %q",
				sender, *stateKey,
			)
		}
	}

	// TODO: Implement other restrictions on state_keys required by the specification.
	// However as synapse doesn't implement those checks at the moment we'll hold off
	// so that checks between the two codebases don't diverge too much.

	return nil
}

// A membershipAllower has the information needed to authenticate a m.room.member event
type membershipAllower struct {
	// The user ID of the user whose membership is changing.
	targetID string
	// The user ID of the user who sent the membership event.
	senderID string
	// The membership of the user who sent the membership event.
	senderMember memberContent
	// The previous membership of the user whose membership is changing.
	oldMember memberContent
	// The new membership of the user if this event is accepted.
	newMember memberContent
	// The m.room.create content for the room.
	create createContent
	// The m.room.power_levels content for the room.
	powerLevels powerLevelContent
	// The m.room.join_rules content for the room.
	joinRule joinRuleContent
}

// newMembershipAllower loads the information needed to authenticate the m.room.member event
// from the auth events.
func newMembershipAllower(authEvents AuthEvents, event Event) (m membershipAllower, err error) {
	stateKey := event.StateKey()
	if stateKey == nil {
		err = errorf("m.room.member must be a state event")
		return
	}
	// TODO: Check that the IDs are valid user IDs.
	m.targetID = *stateKey
	m.senderID = event.Sender()
	if m.create, err = newCreateContentFromAuthEvents(authEvents); err != nil {
		return
	}
	if m.newMember, err = newMemberContentFromEvent(event); err != nil {
		return
	}
	if m.oldMember, err = newMemberContentFromAuthEvents(authEvents, m.targetID); err != nil {
		return
	}
	if m.senderMember, err = newMemberContentFromAuthEvents(authEvents, m.senderID); err != nil {
		return
	}
	if m.powerLevels, err = newPowerLevelContentFromAuthEvents(authEvents, m.create.Creator); err != nil {
		return
	}
	// We only need to check the join rules if the proposed membership is "join".
	if m.newMember.Membership == "join" {
		if m.joinRule, err = newJoinRuleContentFromAuthEvents(authEvents); err != nil {
			return
		}
	}
	return
}

// membershipAllowed checks whether the membership event is allowed
func (m *membershipAllower) membershipAllowed(event Event) error {
	if m.create.roomID != event.RoomID() {
		return errorf("create event has different roomID: %q != %q", event.RoomID(), m.create.roomID)
	}
	if err := m.create.userIDAllowed(m.senderID); err != nil {
		return err
	}
	if err := m.create.userIDAllowed(m.targetID); err != nil {
		return err
	}
	// Special case the first join event in the room to allow the creator to join.
	// https://github.com/matrix-org/synapse/blob/v0.18.5/synapse/api/auth.py#L328
	if m.targetID == m.create.Creator &&
		m.newMember.Membership == join &&
		m.senderID == m.targetID &&
		len(event.PrevEvents()) == 1 {

		// Grab the event ID of the previous event.
		prevEventID := event.PrevEvents()[0].EventID

		if prevEventID == m.create.eventID {
			// If this is the room creator joining the room directly after the
			// the create event, then allow.
			return nil
		}
		// Otherwise fall back to the normal checks.
	}

	if m.newMember.Membership == invite && len(m.newMember.ThirdPartyInvite) != 0 {
		// Special case third party invites
		// https://github.com/matrix-org/synapse/blob/v0.18.5/synapse/api/auth.py#L393
		panic(fmt.Errorf("ThirdPartyInvite not implemented"))
	}

	if m.targetID == m.senderID {
		// If the state_key and the sender are the same then this is an attempt
		// by a user to update their own membership.
		return m.membershipAllowedSelf()
	}
	// Otherwise this is an attempt to modify the membership of somebody else.
	return m.membershipAllowedOther()
}

// membershipAllowedSelf determines if the change made by the user to their own membership is allowed.
func (m *membershipAllower) membershipAllowedSelf() error {
	if m.newMember.Membership == join {
		// A user that is not in the room is allowed to join if the room
		// join rules are "public".
		if m.oldMember.Membership == leave && m.joinRule.JoinRule == public {
			return nil
		}
		// An invited user is allowed to join if the join rules are "public"
		if m.oldMember.Membership == invite && m.joinRule.JoinRule == public {
			return nil
		}
		// An invited user is allowed to join if the join rules are "invite"
		if m.oldMember.Membership == invite && m.joinRule.JoinRule == invite {
			return nil
		}
		// A joined user is allowed to update their join.
		if m.oldMember.Membership == join {
			return nil
		}
	}
	if m.newMember.Membership == leave {
		// A joined user is allowed to leave the room.
		if m.oldMember.Membership == join {
			return nil
		}
		// An invited user is allowed to reject an invite.
		if m.oldMember.Membership == invite {
			return nil
		}
	}
	return m.membershipFailed()
}

// membershipAllowedOther determines if the user is allowed to change the membership of another user.
func (m *membershipAllower) membershipAllowedOther() error {
	senderLevel := m.powerLevels.userLevel(m.senderID)
	targetLevel := m.powerLevels.userLevel(m.targetID)

	// You may only modify the membership of another user if you are in the room.
	if m.senderMember.Membership != join {
		return errorf("sender %q is not in the room", m.senderID)
	}

	if m.newMember.Membership == ban {
		// A user may ban another user if their level is high enough
		// https://github.com/matrix-org/synapse/blob/v0.18.5/synapse/api/auth.py#L463
		if senderLevel >= m.powerLevels.banLevel &&
			senderLevel > targetLevel {
			return nil
		}
	}
	if m.newMember.Membership == leave {
		// A user may unban another user if their level is high enough.
		// This is doesn't require the same power_level checks as banning.
		// You can unban someone with higher power_level than you.
		// https://github.com/matrix-org/synapse/blob/v0.18.5/synapse/api/auth.py#L451
		if m.oldMember.Membership == ban && senderLevel >= m.powerLevels.banLevel {
			return nil
		}
		// A user may kick another user if their level is high enough.
		// TODO: You can kick a user that was already kicked, or has left the room, or was
		// never in the room in the first place. Do we want to allow these redundant kicks?
		if m.oldMember.Membership != ban &&
			senderLevel >= m.powerLevels.kickLevel &&
			senderLevel > targetLevel {
			return nil
		}
	}
	if m.newMember.Membership == invite {
		// A user may invite another user if the user has left the room.
		// and their level is high enough.
		if m.oldMember.Membership == leave && senderLevel >= m.powerLevels.inviteLevel {
			return nil
		}
		// A user may re-invite a user.
		if m.oldMember.Membership == invite && senderLevel >= m.powerLevels.inviteLevel {
			return nil
		}
	}

	return m.membershipFailed()
}

// membershipFailed returns a error explaining why the membership change was disallowed.
func (m *membershipAllower) membershipFailed() error {
	if m.senderID == m.targetID {
		return errorf(
			"%q is not allowed to change their membership from %q to %q",
			m.targetID, m.oldMember.Membership, m.newMember.Membership,
		)
	}

	return errorf(
		"%q is not allowed to change the membership of %q from %q to %q",
		m.senderID, m.targetID, m.oldMember.Membership, m.newMember.Membership,
	)
}
