# Lộ trình

[English](../roadmap.md) · **Tiếng Việt**

Trong thời gian tới, trọng tâm là cải thiện bản xem trước Android và host, thu hẹp các khoảng trống riêng từng runtime và làm rõ hành vi được hỗ trợ. Thứ tự cụ thể có thể thay đổi trong quá trình phát triển; xem [trạng thái hiện tại](status.md) để biết phạm vi đã có.

## Định hướng tương lai

Đây chưa phải tính năng khả dụng hay cam kết thương mại cuối cùng:

| Định hướng | Ranh giới dự kiến |
| --- | --- |
| **Basic** | Relay tới một máy tính của người dùng mà không cần tự cấu hình VPN hay mở port router, một agent đang chạy, giới hạn upload từ điện thoại và thông báo đẩy cơ bản. Máy tính của người dùng vẫn là host chạy agent và người dùng tự cung cấp tài khoản agent. |
| **Plus** | Cùng mô hình chạy trên máy người dùng, hỗ trợ tối đa ba host kết nối, tối đa năm agent độc lập đang chạy, giới hạn upload lớn hơn và thông báo linh hoạt hơn. Không có điều phối tự động giữa các agent. |
| **Pro** | Máy thực thi do Modex quản lý để máy tính cá nhân có thể tắt, kèm điều phối agent qua nhiều provider cho các vai trò như lập kế hoạch, triển khai, review và deploy. |

Sản phẩm local/SSH hiện tại vẫn hữu ích mà không cần Modex Cloud. Basic và Plus là mô hình relay, không phải môi trường chạy agent được host sẵn. Pro là mô hình thực thi được quản lý. Thông báo đẩy thuộc định hướng cloud này; bản xem trước hiện tại không phụ thuộc FCM cho kết nối SSH thông thường khi ứng dụng đang mở.

Giá, thanh toán, hạn mức, cách xử lý tài khoản provider và điều khoản thương mại chưa được quyết định. Xem [tổng quan thiết kế](../design.md) để biết định hướng sản phẩm hiện tại.
