rm token.tar.gz 2>/dev/null
peer lifecycle chaincode package token.tar.gz --path . --lang golang --label token.8
