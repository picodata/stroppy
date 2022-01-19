local errors = require('errors')
local not_found_error = errors.new_class("NotFoundError")
local conflict_error = errors.new_class("ConflictError")


local custom_errors = {
    storageConflictErrors = {
        AccountAlReadyExist = conflict_error:new("Account already exist"),
        TransferAlReadyExist = conflict_error:new("Transfer already exist"),
        SetingsAlreadyExist = conflict_error:new("Settings already exist"),
        SettingsIncorrectCount = conflict_error:new("Settings found, expected 2 parameters, but got another count")
},

    storageNotFoundErrors = {
        AccNotFound = not_found_error:new("Account not found"),
        SettingsNotFound = not_found_error:new("Settings not found"),
        totalBalanceNotFound = not_found_error:new("Total balance not found"),
}
}

return custom_errors