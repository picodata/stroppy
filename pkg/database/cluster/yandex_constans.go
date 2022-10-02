package cluster

const (
	yqlInsertAccount = `
DECLARE $bic AS String; DECLARE $ban AS String; DECLARE $balance AS Int64;
INSERT INTO "&{stroppyDir}/account" (bic, ban, balance) VALUES ($bic, $ban, $balance);
`

	yqlUpsertTransfer = `
DECLARE $transfer_id AS String;
DECLARE $src_bic AS String;
DECLARE $src_ban AS String;
DECLARE $dst_bic AS String;
DECLARE $dst_ban AS String;
DECLARE $amount AS Int64;
DECLARE $state AS String;
UPSERT INTO "&{stroppyDir}/transfer" (
    transfer_id,
    src_bic,
    src_ban,
    dst_bic,
    dst_ban,
    amount,
    state
)
VALUES (
    $transfer_id,
    $src_bic,
    $src_ban,
    $dst_bic,
    $dst_ban,
    $amount,
    $state
);`

	yqlSelectSrcDstAccount = `
DECLARE $src_bic AS String;
DECLARE $src_ban AS String;
DECLARE $dst_bic AS String;
DECLARE $dst_ban AS String;
SELECT 1 AS srcdst, balance
FROM "&{stroppyDir}/account"
WHERE bic = $src_bic AND ban = $src_ban
UNION ALL
SELECT 2 AS srcdst, balance
FROM "&{stroppyDir}/account"
WHERE bic = $dst_bic AND ban = $dst_ban;
`

	yqlUpsertSrcDstAccount = `
DECLARE $src_bic AS String;
DECLARE $src_ban AS String;
DECLARE $dst_bic AS String;
DECLARE $dst_ban AS String;
DECLARE $amount AS Int64;
$shared_select = (
    SELECT 
        bic,
        ban,
        balance - $amount AS balance
    FROM "&{stroppyDir}/account"
    WHERE bic = $src_bic AND ban = $src_ban
    UNION ALL
    SELECT 
        bic,
        ban,
        balance + $amount AS balance
    FROM "&{stroppyDir}/account"
    WHERE bic = $dst_bic AND ban = $dst_ban
);

UPDATE "&{stroppyDir}/account" ON
SELECT * FROM $shared_select;
`

	yqlSelectBalanceAccount = `
DECLARE $bic AS String; DECLARE $ban AS String;
SELECT balance, CAST(0 AS Int64) AS pending
FROM "&{stroppyDir}/account"
WHERE bic = $bic AND ban = $ban
`
)
