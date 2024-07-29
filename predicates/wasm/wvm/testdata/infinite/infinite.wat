(module
  (type (;0;) (func (result i32)))
  (func (;0;) (type 0) (result i32)
    loop  ;; label = @1
      br 0 (;@1;)
    end
    unreachable)
  (memory (;0;) 1)
  (export "memory" (memory 0))
  (export "ab_main" (func 0)))
