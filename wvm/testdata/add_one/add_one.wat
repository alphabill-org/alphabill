(module
  (type (;0;) (func (param i32) (result i32)))
  (import "env" "memory" (memory (;0;) 1 20))
  (func $add_one (type 0) (param i32) (result i32)
    local.get 0
    i32.const 1
    i32.add)
  (export "add_one" (func $add_one)))
  
