package cluster

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_expandYql(t *testing.T) {
	for _, tt := range []struct {
		in  string
		out string
	}{
		{
			in: yqlInsertAccount,
			//nolint:lll
			out: "DECLARE $bic AS String; DECLARE $ban AS String; DECLARE $balance AS Int64;\nINSERT INTO `stroppy/account` (bic, ban, balance) VALUES ($bic, $ban, $balance);\n",
		},
		{
			in: yqlTransfer,
			//nolint:lll
			out: "DECLARE $transfer_id AS String;\nDECLARE $src_bic AS String;\nDECLARE $src_ban AS String;\nDECLARE $dst_bic AS String;\nDECLARE $dst_ban AS String;\nDECLARE $amount AS Int64;\nDECLARE $state AS String;\n\n$shared_select = (\n    SELECT\n        bic,\n        ban,\n        Ensure(balance - $amount, balance >= $amount, 'INSUFFICIENT_FUNDS') AS balance\n    FROM `stroppy/account`\n    WHERE bic = $src_bic AND ban = $src_ban\n    UNION ALL\n    SELECT\n        bic,\n        ban,\n        balance + $amount AS balance\n    FROM `stroppy/account`\n    WHERE bic = $dst_bic AND ban = $dst_ban\n);\n\nDISCARD SELECT Ensure(2, cnt=2, 'MISSING_ACCOUNTS')\nFROM (SELECT COUNT(*) AS cnt FROM $shared_select);\n\nUPSERT INTO `stroppy/account`\nSELECT * FROM $shared_select;\n\nUPSERT INTO `stroppy/transfer` (transfer_id, src_bic, src_ban, dst_bic, dst_ban, amount, state)\nVALUES ($transfer_id, $src_bic, $src_ban, $dst_bic, $dst_ban, $amount, $state);\n",
		},
		{
			in: yqlSelectBalanceAccount,
			//nolint:lll
			out: "DECLARE $bic AS String; DECLARE $ban AS String;\n\nSELECT balance, CAST(0 AS Int64) AS pending\nFROM `stroppy/account`\nWHERE bic = $bic AND ban = $ban\n",
		},
	} {
		t.Run("", func(t *testing.T) {
			require.Equal(t, tt.out, expandYql(tt.in))
		})
	}
}
