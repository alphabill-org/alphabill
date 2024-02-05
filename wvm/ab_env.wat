;; module should be exported as "env".
;;
;; This should be regenerated via `wat2wasm --debug-names env.wat`
;;
(module $env

;; ┌──────────────────────────────────────────────────────────────────────────┐
;; │                                                                          │
;; │ Memory functions                                                         │
;; │                                                                          │
;; └──────────────────────────────────────────────────────────────────────────┘

  ;; free
  ;;
  ;; Note: This re-exports the same function from the "ab" module.
  (func $free (export "ext_free") (import "ab" "ext_free")
    (param $addr i32))

  ;; malloc
  ;;
  ;; Note: This re-exports the same function from the "ab" module.
  (func $malloc (export "ext_malloc") (import "ab" "ext_malloc")
    (param $size i32)
    (result (; $addr ;)i32))

;; ┌──────────────────────────────────────────────────────────────────────────┐
;; │                                                                          │
;; │ Other functions                                                          │
;; │                                                                          │
;; └──────────────────────────────────────────────────────────────────────────┘
  ;; p2pkh
  ;;
  ;; Note: This re-exports the same function from the "ab" module.
  (func $p2pkh (export "p2pkh_v1") (import "ab" "p2pkh_v1")
    (result (; $result ;)i32))

  ;; p2pkh_v2
  ;;
  ;; Note: This re-exports the same function from the "ab" module.
  (func $p2pkh_v2 (export "p2pkh_v2") (import "ab" "p2pkh_v2")
    (param $pub_key_hash i64)
    (result (; $result ;)i32))

  ;; storage read
  ;;
  ;; Note: This re-exports the same function from the "ab" module.
  (func $storage_read (export "storage_read") (import "ab" "storage_read")
    (param $id i32)
    (result (; $result ;)i64))

  ;; storage write
  ;;
  ;; Note: This re-exports the same function from the "ab" module.
  (func $storage_write (export "storage_write") (import "ab" "storage_write")
    (param $id i32)
    (param $addr i64)
    (result (; $result ;)i32))

  ;; log_v1
  ;;
  ;; Note: This re-exports the same function from the "ab" module.
  (func $logStr (export "log_v1") (import "ab" "log_v1")
    (param $level i32)
    (param $textPtr i64))

  ;; export memory (1 64KB page at start but can grow to 20*64KB) to guest
  (memory (export "memory") 1 20)
)

