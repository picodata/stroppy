package cluster

const (
	SumAccountsBalancesFunction = `
	function sum_accounts_balances()
   local v, t, sum
   sum = 0
   for v, t in box.space.accounts:pairs() do 
      print(t[3])
      if type(t[3]) == "number" then sum = sum + t[3]
      end
   end
   return sum
end`

	MakeAtomicTransferFunction = `
function makeAtomicTransfer(id, src_bic,src_ban, dest_bic, dest_ban, amount)
  uuid = require('uuid')
  box.begin()
  local new_source = box.space.accounts:update({src_bic,src_ban},{{'-', 3, amount}})
  if new_source == nil then
  	box.rollback()
  	return "ErrNotFound"
  end
  if new_source[3]<0 then
        box.rollback()
  	return "ErrInsufficientFunds" 
  end
  local new_dest = box.space.accounts:update({dest_bic,dest_ban}, {{'+', 3, amount}})
  if new_dest == nil then
  	box.rollback()
  	return "ErrNotFound"
  end
  box.space.transfers:insert({uuid.fromstr(id), src_bic,src_ban, dest_bic, dest_ban, amount})       
  box.commit()
  return "ok"
end`
)
