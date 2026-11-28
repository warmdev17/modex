# Hỗ trợ agent

[English](../agents.md) · **Tiếng Việt**

Phạm vi hỗ trợ khác nhau theo từng runtime. Modex không tuyên bố các agent có bộ tính năng tương đương. Hãy cài và đăng nhập runtime trên host trước khi dùng; bridge đang chạy không chứng minh provider đã xác thực hay inference hoạt động.

## Kết nối agent

1. Cài runtime trên host và đăng nhập provider theo quy trình bình thường của runtime đó.
2. Cài Modex rồi khởi động host service cho workspace. Service mặc định dùng Codex; OpenCode cần cấu hình backend riêng theo [hướng dẫn backend](../opencode-backend.md).
3. Bật đăng nhập SSH bằng public key, kiểm tra fingerprint của host key bên ngoài Modex, sau đó thêm host vào ứng dụng Android.
4. Mở hoặc tạo project rồi chọn runtime khi tạo chat mới. Antigravity chỉ xuất hiện trên kết nối dùng Codex sau khi host xác nhận CLI, đăng nhập và khả năng khôi phục phiên ACP đã sẵn sàng.

Chat hiện có tiếp tục dùng runtime và phiên native đã chọn. Kết nối host không khóa toàn bộ project hay chat vào một agent.

| Runtime | Có trong bản xem trước này | Giới hạn đã biết |
| --- | --- | --- |
| Codex | Đường tích hợp đầy đủ nhất: chat, streaming, lịch sử, tệp đính kèm, phê duyệt/câu hỏi được hỗ trợ và các điều khiển riêng Codex mà host công bố. | Một số khả năng và thao tác native của Codex chưa có; xem [tài liệu protocol](../history-contract.md) và [thiết kế](../design.md). |
| OpenCode | Chat, lịch sử, chọn model, tệp đính kèm, ngữ cảnh project, form và trạng thái công cụ. | Chưa có plan mode, permission profile, takeover, reference hay toàn bộ điều khiển project riêng Codex. Trạng thái phiên chỉ tồn tại theo supervisor. |
| Antigravity | Prompt văn bản, streaming, Stop, lịch sử, quyền dùng một lần và chọn model, mức suy nghĩ trên host tương thích. | Đính kèm và gửi thêm prompt cần bản phát triển mô tả bên dưới. |

Antigravity cần tải được danh sách model và hỗ trợ khôi phục phiên qua
ACP thì mới chọn được. Codex và OpenCode cần khởi tạo được bridge. Khi
gửi prompt, mỗi provider kiểm tra tài khoản và quota theo quy định riêng.

Ma trận này mô tả bản xem trước hiện tại. [Snapshot triển khai backend OpenCode](../opencode-backend.md) ghi lại một revision đã kiểm chứng trước đây và có thể chưa gồm phần project, form hay lịch sử tool mới hơn.

## Chat nhiều agent trong bản phát triển

### Goal

Trong chat Codex với host và APK mới, mở menu chat rồi chọn **Goal**.
Modal cho nhập mục tiêu và ngân sách token tùy chọn, đồng thời xem trạng
thái, token đã dùng và thời gian. Pause dừng việc tự tiếp tục goal;
muốn ngắt lượt đang chạy thì dùng Stop. Resume cho phép Codex tiếp tục
làm việc. Thay đổi goal từ CLI được đồng bộ, mở lại modal sẽ đọc trạng
thái mới từ host.
Có thể sửa mục tiêu hoặc ngân sách của goal hiện tại. Khi hết ngân sách,
nhập mức mới cao hơn lượng đã dùng rồi chọn **Update & resume**.
Mở lại goal Active có thể tiếp tục chạy như Codex CLI.

Mốc đầu dùng API goal của Codex 0.169.0. OpenCode 2.4.1 có goal gốc với
pause/resume/clear nhưng Modex chưa nối API đó. Antigravity 1.5.17 chưa
công bố khả năng goal qua ACP. Vì vậy modal Goal chỉ bật cho chat Codex
trong mốc này. Chat OpenCode và Antigravity vẫn dùng các chức năng hiện có.

### Chọn runtime

Khi đã cập nhật cả host và Android, kết nối SSH một lần rồi chọn Codex,
OpenCode hoặc Antigravity trong **New chat**. Có thể lọc danh sách theo
agent. Các chat cùng project giữ agent và lịch sử riêng.

Đường OpenCode do Modex quản lý cần OpenCode từ 2.4 trong dòng 2.x. Nó
dùng cơ sở dữ liệu riêng trong thư mục trạng thái supervisor. Chat và
tài khoản OpenCode độc lập vẫn nằm trong hồ sơ hiện có. Hãy cấu hình
quyền truy cập provider trên host và kiểm tra model cần dùng trong hồ sơ
được quản lý trước khi làm việc.

Để đăng nhập OpenCode tương tác, đặt `supervisor_state` thành thư mục
trạng thái đã cấu hình trong Modex. Chạy bằng chính tài khoản người dùng
trên host, sau khi lần kiểm tra runtime đầu tiên đã tạo hồ sơ.

```sh
profile="$supervisor_state/agent-chats/opencode"
XDG_DATA_HOME="$profile/data" XDG_STATE_HOME="$profile/state" \
  OPENCODE_DB="$profile/data/opencode/opencode.db" opencode auth login --standalone
```

Làm mới danh sách agent sau khi đăng nhập.

## Dùng chung phiên trên terminal

Bộ chọn CLI hiện icon và kiểm tra từng agent độc lập. Có thể chọn Codex
khi AGY/OpenCode vẫn đang kiểm tra. Danh sách chat chỉ thuộc thư mục chạy
`modex cli` hoặc `--workspace` đã chỉ định, có phân giải đường dẫn symlink.
Đường dẫn project luôn hiện trong bộ chọn. Quay lại trang đã tải sẽ dùng
cache trong lần mở CLI đó; bấm `r` để tải lại session hoặc kiểm tra lại agent
đang chọn. Lỗi tải giữ nguyên màn hình để thử lại. Terminal không dùng
UTF-8 sẽ hiện icon ASCII.

Chạy `modex cli` trong terminal. Dùng ↑/↓ để chọn agent và phiên, Enter
để mở, Esc để quay lại. Chat mới dùng workspace hiện tại. Codex mở giao
diện terminal riêng.

OpenCode và Antigravity mở giao diện terminal gốc đã cài trên host.
OpenCode kết nối vào đúng server và phiên native do Modex quản lý.
Những tài khoản provider còn thiếu được nhập từ OpenCode trên host bằng
lệnh export/import của OpenCode. Tài khoản đã có trong hồ sơ Modex giữ
lựa chọn hiện tại.

Khi TUI OpenCode mở, Modex theo dõi các lượt bắt đầu từ terminal để
Android nhận hoạt động và trả lời yêu cầu đang chờ. Thoát TUI dừng theo
dõi lượt mới, các lượt đã nhận vẫn tiếp tục chạy. Antigravity mở đúng
cuộc trò chuyện native đã lưu và giữ phiên cho terminal. Đóng terminal
đó trước khi mở lại cùng chat trên Android để tải lịch sử và gửi prompt.

Giao diện dùng màu RGB theo nền terminal và hỗ trợ `NO_COLOR`. Khi đọc
từ pipe, CLI giữ giao diện theo dòng. Mở thẳng phiên đã lưu bằng
`modex cli --thread <id>`. Với OpenCode hoặc Antigravity, chạy từ
workspace của phiên hoặc chỉ định `--workspace`.

## Antigravity trong bản phát triển

Modex chạy terminal Antigravity, kiểm tra model và ACP với
`AGY_CLI_FORCE_FILE_TOKEN_STORAGE=1`. Các tiến trình dùng chung token
AGY lưu trên file qua những lần mở shell và host mới. Nếu chưa từng
đăng nhập vào kho này, chạy một lần bằng tài khoản của host, rồi mở
lại bộ chọn.

```sh
AGY_CLI_FORCE_FILE_TOKEN_STORAGE=1 agy acp --login
```

Khi cả host và Android hỗ trợ tùy chọn phiên Antigravity, bộ chọn trong
chat hiển thị các model và mức suy nghĩ do ACP cung cấp. Có thể đổi khi chat
đang rảnh. Modex lưu lựa chọn đã được xác nhận theo từng chat và đối chiếu
với các tùy chọn còn khả dụng khi mở lại. Host cũ và phiên ACP thiếu tùy
chọn tiếp tục dùng model mặc định của Antigravity.
Antigravity 1.5.17 đưa chat mới vào danh sách khôi phục sau lượt chat đầu
tiên được ghi lại.

Trên bản tương thích, đầu chat và Session status hiển thị dung lượng
context do Antigravity báo về. Số token cập nhật theo dữ liệu từ CLI và
có thể cần đợi mẫu mới sau khi mở lại phiên native.

Khi host đã cập nhật, lệnh shell do Antigravity báo về xuất hiện trong
cuộc trò chuyện. Mở thẻ lệnh để xem output và mã thoát nếu có. Khi mở lại
chat, Modex hiển thị các lệnh được Antigravity trả về trong lịch sử.

Khi Antigravity báo đã sửa file và gửi nội dung trước và sau khi sửa,
mở thẻ sửa file để xem các dòng thay đổi. Khi mở lại chat, Modex hiển thị
các bản sửa có trong lịch sử do Antigravity trả về. File quá lớn có thể
không xuất hiện, còn bản diff bị cắt sẽ có dòng báo trong thẻ.

Khi cả host lẫn Android đã cập nhật, tên do Antigravity tạo sẽ hiện ở
đầu chat và trong sidebar, kể cả sau khi kết nối lại. Chat cũ lấy lại
tên khi mở nếu Antigravity cung cấp tên đó.

Khi cả host lẫn Android đã cập nhật, dùng **Attach files** trong chat
Antigravity để gửi ảnh PNG/JPEG hoặc file văn bản UTF-8. Có thể gửi file
riêng hoặc kèm lời nhắn. Mỗi tin nhắn gửi tối đa tám file, tổng 8 MiB.
Mỗi file văn bản tối đa 256 KiB. Khả năng nhận ảnh còn phụ thuộc model
đang dùng. Sau khi host khởi động lại, Modex khôi phục phần xem trước từ
các đường dẫn file mà Antigravity trả về trong lịch sử. Một số file đã gửi
có thể không xuất hiện lại.

Với bản Modex host và Android tương thích cùng Antigravity CLI 1.5.17,
bấm Send lúc agent đang trả lời để gửi thêm prompt văn bản vào cùng
phiên. Đợi phiên rảnh để gửi nháp có file đính kèm. Stop dừng lượt đang
chạy nếu còn prompt chưa hoàn tất.

Bảng Agents hiển thị lượt gọi subagent do Antigravity báo về. Trạng thái
hoàn tất của lượt gọi nghĩa là công cụ đã trả kết quả. Bản CLI được kiểm
thử cung cấp trạng thái lượt gọi, còn lịch sử riêng của subagent chưa có.
