# スマートメータクライアント

## 使用方法

1. BP35A1をUSBで接続する。

1. プログラムを実行する
    ```
    go run cmd/main.go -p <port name> -i <B route id>0 -P <B route password>
    ```

出力例
```
version:  EVER 1.2.10
finish regist channel
finish regist pan id
finish regist pan id
瞬時電力量: 407 W
計測時間:  2023-07-10 02:00:00 +0000 UTC
積算電力量: 5847.800000
2023-07-10 02:18:42.509018 +0900 JST m=+15.803980335 1
```