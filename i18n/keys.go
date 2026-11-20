package i18n

const MsgTaskPluginUnknownMetaField = "task_plugin.unknown_meta_field"

// Message keys for i18n translations
// Use these constants instead of hardcoded strings

// Common error messages
const (
	MsgInvalidParams = "common.invalid_params"
	MsgDatabaseError = "common.database_error"
	MsgUpdateSuccess = "common.update_success"
	MsgUpdateFailed  = "common.update_failed"
	MsgBatchTooMany  = "common.batch_too_many"
)

// Auth middleware messages
const (
	MsgAuthNotLoggedIn           = "auth.not_logged_in"
	MsgAuthAccessTokenInvalid    = "auth.access_token_invalid"
	MsgAuthUserInfoInvalid       = "auth.user_info_invalid"
	MsgAuthUserBanned            = "auth.user_banned"
	MsgAuthInsufficientPrivilege = "auth.insufficient_privilege"
)

// Token related messages
const (
	MsgTokenNameTooLong          = "token.name_too_long"
	MsgTokenQuotaNegative        = "token.quota_negative"
	MsgTokenQuotaExceedMax       = "token.quota_exceed_max"
	MsgTokenGenerateFailed       = "token.generate_failed"
	MsgTokenGetInfoFailed        = "token.get_info_failed"
	MsgTokenExpiredCannotEnable  = "token.expired_cannot_enable"
	MsgTokenExhaustedCannotEable = "token.exhausted_cannot_enable"
	MsgTokenInvalid              = "token.invalid"
	MsgTokenNotProvided          = "token.not_provided"
	MsgTokenStatusUnavailable    = "token.status_unavailable"
	MsgTokenAutoGroupsTooMany    = "token.auto_groups_too_many"
	MsgTokenAutoGroupsDuplicate  = "token.auto_groups_duplicate"
	MsgTokenAutoGroupsInvalid    = "token.auto_groups_invalid"
)

// User related messages
const (
	MsgUserPasswordLoginDisabled     = "user.password_login_disabled"
	MsgUserRegisterDisabled          = "user.register_disabled"
	MsgUserPasswordRegisterDisabled  = "user.password_register_disabled"
	MsgUserUsernameOrPasswordError   = "user.username_or_password_error"
	MsgUserExists                    = "user.exists"
	MsgUserNotExists                 = "user.not_exists"
	MsgUserEmailVerificationRequired = "user.email_verification_required"
	MsgUserVerificationCodeError     = "user.verification_code_error"
	MsgUserEmailAlreadyTaken         = "user.email_already_taken"
	MsgUserPasswordResetLinkInvalid  = "user.password_reset_link_invalid"
	MsgUserInputInvalid              = "user.input_invalid"
	MsgUserNoPermissionSameLevel     = "user.no_permission_same_level"
	MsgUserNoPermissionHigherLevel   = "user.no_permission_higher_level"
	MsgUserCannotCreateHigherLevel   = "user.cannot_create_higher_level"
	MsgUserCannotDeleteRootUser      = "user.cannot_delete_root_user"
	MsgUserCannotDisableRootUser     = "user.cannot_disable_root_user"
	MsgUserCannotDemoteRootUser      = "user.cannot_demote_root_user"
	MsgUserAlreadyAdmin              = "user.already_admin"
	MsgUserAlreadyCommon             = "user.already_common"
	MsgUserAdminCannotPromote        = "user.admin_cannot_promote"
	MsgUserRegisterFailed            = "user.register_failed"
	MsgUserDefaultTokenFailed        = "user.default_token_failed"
	MsgUserQuotaChangeZero           = "user.quota_change_zero"
)

// Quota related messages
const (
	MsgQuotaThresholdGtZero = "quota.threshold_gt_zero"
)

// Channel related messages
const (
	MsgChannelNotExists = "channel.not_exists"
)

// Passkey related messages
const (
	MsgPasskeyRPIDInvalid             = "passkey.rp_id_invalid"
	MsgPasskeyRPIDUnavailable         = "passkey.rp_id_unavailable"
	MsgPasskeyRPIDRemovalConfirmation = "passkey.rp_id_removal_confirmation"
)

// Setting related messages
const (
	MsgSettingInvalidType  = "setting.invalid_type"
	MsgSettingEmailInvalid = "setting.email_invalid"
	MsgSettingSaved        = "setting.saved"
)

// Model layer error messages (for translation in controller)
const (
	MsgCreateDefaultTokenErr = "user.create_default_token_error"
	MsgUuidDuplicate         = "common.uuid_duplicate"
	MsgInvalidInput          = "common.invalid_input"
)

// Distributor related messages
const (
	MsgDistributorInvalidRequest      = "distributor.invalid_request"
	MsgDistributorInvalidChannelId    = "distributor.invalid_channel_id"
	MsgDistributorChannelDisabled     = "distributor.channel_disabled"
	MsgDistributorTokenNoModelAccess  = "distributor.token_no_model_access"
	MsgDistributorTokenModelForbidden = "distributor.token_model_forbidden"
	MsgDistributorModelNameRequired   = "distributor.model_name_required"
	MsgDistributorInvalidPlayground   = "distributor.invalid_playground_request"
	MsgDistributorGroupAccessDenied   = "distributor.group_access_denied"
	MsgDistributorGetChannelFailed    = "distributor.get_channel_failed"
	MsgDistributorNoAvailableChannel  = "distributor.no_available_channel"
	MsgDistributorModelUnavailable    = "distributor.model_unavailable"
)
