(module
  (func $f (param i32)
    block
      local.get 0
      i32.eqz
      br_if 0
      local.get 0
      i32.const 1
      i32.sub
      call $f
    end
  )
  (export "f" (func $f))
)