;; module should be exported as "env".
;;
;; This should be regenerated via `wat2wasm --debug-names env.wat`
;;
(module $env
  ;; export memory (1 64KB page at start but can grow to 20*64KB) to guest
  (memory (export "memory") 1 20)
)