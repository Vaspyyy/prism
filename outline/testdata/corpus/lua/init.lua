local M = {}

--[[
function M.commented_out()
end
]]

local help = [[
function not_a_function()
end
]]

local other = [==[
function also_not()
]==]

function M.setup(opts)
  M.opts = opts
end

local function helper()
  return help
end

function M:method()
  return helper()
end

return M
