# markdownlint config
# see https://github.com/markdownlint/markdownlint/tree/main
all

# logo
exclude_rule 'MD033'
# not working with leading ? titles
exclude_rule 'MD026'
# markdown metadata
exclude_rule 'MD041'

# unordered list indentation 2 spaces
rule 'MD007', :indent => 2
# increase line length
rule 'MD013', :line_length => 120, :ignore_code_blocks => true, :tables => false
# allow ordered lists
rule 'MD029', :style => 'ordered'
# allow duplicate headers titles only in different nestings
rule 'MD024', :allow_different_nesting => true
