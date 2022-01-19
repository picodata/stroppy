local t = require("luatest")
local g = t.group("integration_api")
local fio = require('fio')
local decimal = require("decimal")

local helper = require("test.helper")
local cartridge_helpers = require("cartridge.test-helpers")
local shared = require("test.helper")

local function assert_http_request(method, path, json, expect)
	local response = g.cluster.main_server:http_request(method, path, { json = json, raise = false })
	t.assert_equals(response.json["info"], expect.info)
	t.assert_equals(response.status, expect.status)
end

g.before_all(function()
	g.cluster = cartridge_helpers.Cluster:new({
		server_command = shared.server_command,
		datadir = shared.datadir,
		use_vshard = true,
		replicasets = {
			{
				alias = "api",
				uuid = cartridge_helpers.uuid("a"),
				roles = { "api" },
				servers = {
					{
						instance_uuid = cartridge_helpers.uuid("a", 1),
						advertise_port = 13301,
						http_port = 8081,
					},
				},
			},
			{
				alias = "storage1",
				uuid = cartridge_helpers.uuid("b"),
				roles = { "storage" },
				servers = {
					{
						instance_uuid = cartridge_helpers.uuid("b", 1),
						advertise_port = 13302,
						http_port = 8082,
					},
				},
			},
			{
				alias = "storage2",
				uuid = cartridge_helpers.uuid("c"),
				roles = { "storage" },
				servers = {
					{
						instance_uuid = cartridge_helpers.uuid("c", 1),
						advertise_port = 13303,
						http_port = 8083,
					},
				},
			},
		},
	})

	g.cluster:start()

end)

g.before_each = function()
	-- helper.truncate_space_on_cluster(g.cluster, 'Set your space name here')
end

g.after_all = function()
	g.cluster:stop() 
end


g.test_bootstrap_db = function ()
    assert_http_request(
		"POST",
		"/db/bootstrap",
		{ count = 10000, seed = 12345678},
		{ info = "Succesfully bootstraping DB", status = 201 }
	)
end

g.test_insert_account = function()
	assert_http_request(
		"POST",
		"/account/insert",
		{ bic = "333", ban = "333", balance = decimal.new(123458), pending_amount = 0},
		{ info = "Successfully created", status = 201 }
	)
end
