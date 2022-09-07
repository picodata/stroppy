package cluster

const (
	insertYdbTransfer = `
DECLARE $params AS Struct<
    transfer_id:String, 
    src_bic:String,
    src_ban:String,
    dst_bic:String,
    dst_ban:String,
    amount:Int64,
    state:String
>;
UPSERT INTO %s (
    transfer_id,
    src_bic,
    src_ban,
    dst_bic,
    dst_ban,
    amount,
    state
)
VALUES (
    $params.transfer_id,
    $params.src_bic,
    $params.src_ban,
    $params.dst_bic,
    $params.dst_ban,
    $params.amount,
    $params.state
);`
	srcAndDstYdbSelect = `
DECLARE $params AS Struct<
    src_bic:String,
    src_ban:String,
    dst_bic:String,
    dst_ban:String,
>;
SELECT 
    bic,
    ban,
    balance
FROM %s
WHERE bic = $params.src_bic AND ban = $params.src_ban
UNION ALL
SELECT 
    bic,
    ban,
    balance
FROM %s
WHERE bic = $params.dst_bic AND ban = $params.dst_ban;
`
	unifiedTransfer = `
DECLARE $params AS Struct<
    src_bic:String,
    src_ban:String,
    dst_bic:String,
    dst_ban:String,
    amount:Int64,
>;
$shared_select = (
    SELECT 
        bic,
        ban,
        balance - $params.amount AS balance
    FROM %s
    WHERE bic = $params.src_bic AND ban = $params.src_ban
    UNION ALL
    SELECT 
        bic,
        ban,
        balance + $params.amount AS balance
    FROM %s
    WHERE bic = $params.dst_bic AND ban = $params.dst_ban
);

UPDATE %s ON
SELECT * FROM $shared_select;
`
)
