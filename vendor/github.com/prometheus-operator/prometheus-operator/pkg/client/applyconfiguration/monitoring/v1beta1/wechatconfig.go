// Copyright The prometheus-operator Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Code generated by applyconfiguration-gen. DO NOT EDIT.

package v1beta1

// WeChatConfigApplyConfiguration represents an declarative configuration of the WeChatConfig type for use
// with apply.
type WeChatConfigApplyConfiguration struct {
	SendResolved *bool                                `json:"sendResolved,omitempty"`
	APISecret    *SecretKeySelectorApplyConfiguration `json:"apiSecret,omitempty"`
	APIURL       *string                              `json:"apiURL,omitempty"`
	CorpID       *string                              `json:"corpID,omitempty"`
	AgentID      *string                              `json:"agentID,omitempty"`
	ToUser       *string                              `json:"toUser,omitempty"`
	ToParty      *string                              `json:"toParty,omitempty"`
	ToTag        *string                              `json:"toTag,omitempty"`
	Message      *string                              `json:"message,omitempty"`
	MessageType  *string                              `json:"messageType,omitempty"`
	HTTPConfig   *HTTPConfigApplyConfiguration        `json:"httpConfig,omitempty"`
}

// WeChatConfigApplyConfiguration constructs an declarative configuration of the WeChatConfig type for use with
// apply.
func WeChatConfig() *WeChatConfigApplyConfiguration {
	return &WeChatConfigApplyConfiguration{}
}

// WithSendResolved sets the SendResolved field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the SendResolved field is set to the value of the last call.
func (b *WeChatConfigApplyConfiguration) WithSendResolved(value bool) *WeChatConfigApplyConfiguration {
	b.SendResolved = &value
	return b
}

// WithAPISecret sets the APISecret field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the APISecret field is set to the value of the last call.
func (b *WeChatConfigApplyConfiguration) WithAPISecret(value *SecretKeySelectorApplyConfiguration) *WeChatConfigApplyConfiguration {
	b.APISecret = value
	return b
}

// WithAPIURL sets the APIURL field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the APIURL field is set to the value of the last call.
func (b *WeChatConfigApplyConfiguration) WithAPIURL(value string) *WeChatConfigApplyConfiguration {
	b.APIURL = &value
	return b
}

// WithCorpID sets the CorpID field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the CorpID field is set to the value of the last call.
func (b *WeChatConfigApplyConfiguration) WithCorpID(value string) *WeChatConfigApplyConfiguration {
	b.CorpID = &value
	return b
}

// WithAgentID sets the AgentID field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the AgentID field is set to the value of the last call.
func (b *WeChatConfigApplyConfiguration) WithAgentID(value string) *WeChatConfigApplyConfiguration {
	b.AgentID = &value
	return b
}

// WithToUser sets the ToUser field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the ToUser field is set to the value of the last call.
func (b *WeChatConfigApplyConfiguration) WithToUser(value string) *WeChatConfigApplyConfiguration {
	b.ToUser = &value
	return b
}

// WithToParty sets the ToParty field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the ToParty field is set to the value of the last call.
func (b *WeChatConfigApplyConfiguration) WithToParty(value string) *WeChatConfigApplyConfiguration {
	b.ToParty = &value
	return b
}

// WithToTag sets the ToTag field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the ToTag field is set to the value of the last call.
func (b *WeChatConfigApplyConfiguration) WithToTag(value string) *WeChatConfigApplyConfiguration {
	b.ToTag = &value
	return b
}

// WithMessage sets the Message field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the Message field is set to the value of the last call.
func (b *WeChatConfigApplyConfiguration) WithMessage(value string) *WeChatConfigApplyConfiguration {
	b.Message = &value
	return b
}

// WithMessageType sets the MessageType field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the MessageType field is set to the value of the last call.
func (b *WeChatConfigApplyConfiguration) WithMessageType(value string) *WeChatConfigApplyConfiguration {
	b.MessageType = &value
	return b
}

// WithHTTPConfig sets the HTTPConfig field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the HTTPConfig field is set to the value of the last call.
func (b *WeChatConfigApplyConfiguration) WithHTTPConfig(value *HTTPConfigApplyConfiguration) *WeChatConfigApplyConfiguration {
	b.HTTPConfig = value
	return b
}