# PHÂN TÍCH LOGIC DUPLICATION DETECTION - PHASE 2

## 📋 TỔNG QUAN QUY TRÌNH

Phase 2 thực hiện việc phát hiện file trùng lặp theo 5 bước chính:

### BƯỚC 1: XÁC ĐỊNH NGHI NGỜ LÀ DUPLICATE (Dựa trên SIZE)

**Query SQL:**
```sql
SELECT size
FROM fs_files
WHERE size > 0 AND hash_value IS NULL
GROUP BY size
HAVING COUNT(*) > 1  -- Chỉ lấy các size có >= 2 files
```

**Logic:**
- Chỉ xử lý các file có `size > 0` và chưa có `hash_value`
- Nhóm theo `size` và chỉ lấy các nhóm có >= 2 files
- **Lý do:** File cùng size có khả năng cao là duplicate (nhưng chưa chắc chắn)

**Ví dụ:**
```
File A: size = 1024 bytes, hash = NULL
File B: size = 1024 bytes, hash = NULL
File C: size = 2048 bytes, hash = NULL
→ Chỉ File A và B được đánh dấu nghi ngờ (cùng size 1024)
→ File C bị bỏ qua (size duy nhất)
```

---

### BƯỚC 2: LẤY TOÀN BỘ FILE CÙNG SIZE

**Query SQL:**
```sql
SELECT f1.id, f1.path
FROM fs_files f1
INNER JOIN (
    SELECT size FROM fs_files
    WHERE size > 0 AND hash_value IS NULL
    GROUP BY size HAVING COUNT(*) > 1
) f2 ON f1.size = f2.size
WHERE f1.size > 0 AND f1.hash_value IS NULL
ORDER BY f1.size
```

**Logic:**
- Lấy tất cả file có cùng size với các nhóm nghi ngờ
- Chỉ lấy file chưa có hash (`hash_value IS NULL`)
- Sắp xếp theo size để xử lý theo nhóm

**Kết quả:**
- Danh sách `FileToHash[]` chứa `{ID, Path}` của tất cả file nghi ngờ

---

### BƯỚC 3: TÍNH HASH CHO CÁC FILE NGHI NGỜ

**Worker Pool:**
```go
for w := 0; w < cfg.MaxWorkers; w++ {
    go func() {
        for job := range jobs {
            hash, err := calculateHashWithContext(ctx, job.Path)
            results <- HashResult{ID: job.ID, Hash: hash, Err: err}
        }
    }()
}
```

**Logic:**
- Sử dụng worker pool để tính hash song song
- Mỗi worker đọc file và tính MD5 hash
- Kết quả được gửi vào channel `results`

**Hash Algorithm:** MD5
- Đọc file theo chunks 64KB
- Hỗ trợ timeout và context cancellation
- Bỏ qua file rỗng (size = 0)

---

### BƯỚC 4: UPDATE HASH VÀO DATABASE

**Update Statement:**
```sql
UPDATE fs_files SET hash_value = ? WHERE id = ?
```

**Logic:**
- Batch processing: Commit mỗi 1000 records
- Sử dụng transaction để đảm bảo consistency
- Chỉ update file có hash hợp lệ (`hash.Valid == true`)

**Ví dụ:**
```
File A (ID=1): hash = "abc123" → UPDATE thành công
File B (ID=2): hash = "abc123" → UPDATE thành công
File C (ID=3): hash = "def456" → UPDATE thành công
```

---

### BƯỚC 5: ĐÁNH DẤU FILE TRÙNG (Hiện tại CHƯA có)

**⚠️ VẤN ĐỀ HIỆN TẠI:**
- Code chỉ update `hash_value` vào DB
- **KHÔNG có bước đánh dấu duplicate ngay sau khi tính hash**
- Việc xác định duplicate chỉ được thực hiện ở phần **REPORT** khi query lại

**Report Query (sau này):**
```sql
SELECT hash_value
FROM fs_files
WHERE hash_value IS NOT NULL
GROUP BY hash_value
HAVING COUNT(*) > 1  -- Tìm hash có >= 2 files
```

---

## 🔍 PHÂN TÍCH CHI TIẾT

### Flow Diagram:

```
PHASE 1: SCAN METADATA
    ↓
[fs_files table]
- id, path, size, hash_value=NULL
    ↓
PHASE 2: DUPLICATION DETECTION
    ↓
Step 1: Tìm size có >= 2 files
    ↓
Step 2: Lấy tất cả file cùng size
    ↓
Step 3: Tính hash (MD5) song song
    ↓
Step 4: Update hash_value vào DB
    ↓
[fs_files table]
- id, path, size, hash_value="abc123"
    ↓
REPORT PHASE (sau này)
    ↓
Query: GROUP BY hash_value HAVING COUNT > 1
    ↓
[Kết quả: Danh sách duplicate groups]
```

---

## ⚠️ VẤN ĐỀ VÀ HẠN CHẾ

### 1. **Không đánh dấu duplicate ngay lập tức**
- Phải query lại ở report phase
- Không có flag `is_duplicate` trong database
- Không biết file nào là duplicate cho đến khi report

### 2. **Xử lý file size duy nhất**
- File có size duy nhất không được tính hash
- Có thể bỏ sót duplicate nếu:
  - File bị xóa sau khi scan
  - File được thêm vào sau khi scan
  - File có size khác nhau nhưng nội dung giống (rất hiếm)

### 3. **Không có thống kê real-time**
- Không biết có bao nhiêu duplicate groups
- Không biết tổng dung lượng duplicate
- Phải chờ đến report phase

---

## 💡 ĐỀ XUẤT CẢI TIẾN

### Option 1: Thêm cột `is_duplicate` vào database

**Schema:**
```sql
ALTER TABLE fs_files ADD COLUMN is_duplicate BOOLEAN DEFAULT 0;
CREATE INDEX idx_file_duplicate ON fs_files(is_duplicate) WHERE is_duplicate = 1;
```

**Logic sau khi tính hash:**
```go
// Sau khi update hash, kiểm tra và đánh dấu duplicate
UPDATE fs_files 
SET is_duplicate = 1 
WHERE hash_value IN (
    SELECT hash_value 
    FROM fs_files 
    WHERE hash_value IS NOT NULL 
    GROUP BY hash_value 
    HAVING COUNT(*) > 1
)
```

### Option 2: Tạo bảng `duplicate_groups` riêng

**Schema:**
```sql
CREATE TABLE duplicate_groups (
    hash_value TEXT PRIMARY KEY,
    file_count INTEGER,
    total_size BIGINT,
    first_seen DATETIME
);

CREATE TABLE duplicate_files (
    file_id INTEGER,
    hash_value TEXT,
    FOREIGN KEY (file_id) REFERENCES fs_files(id),
    FOREIGN KEY (hash_value) REFERENCES duplicate_groups(hash_value)
);
```

**Logic:**
- Sau khi tính hash xong, insert vào `duplicate_groups`
- Link file với group qua `duplicate_files`
- Dễ query và thống kê hơn

### Option 3: Đánh dấu ngay trong quá trình update (Recommended)

**Cải tiến `commitHashBatch`:**
```go
func commitHashBatch(ctx context.Context, db *sql.DB, batch []HashResult, logger *ScannerLogger) int {
    // 1. Update hash_value
    // 2. Sau khi commit, kiểm tra và đánh dấu duplicate ngay
    // 3. Sử dụng một query để update tất cả duplicate cùng lúc
}
```

**Query đánh dấu duplicate:**
```sql
UPDATE fs_files 
SET is_duplicate = 1 
WHERE hash_value IN (
    SELECT hash_value 
    FROM fs_files 
    WHERE hash_value IN (?, ?, ...)  -- Các hash vừa update
    GROUP BY hash_value 
    HAVING COUNT(*) > 1
)
```

---

## 📊 THỐNG KÊ VÀ MONITORING

### Metrics nên track:
1. **Số file nghi ngờ:** Tổng file cùng size
2. **Số file đã hash:** File đã tính hash thành công
3. **Số duplicate groups:** Nhóm file có cùng hash
4. **Tổng dung lượng duplicate:** Tổng size của duplicate files
5. **Thời gian hash:** Thời gian tính hash trung bình

### Logging nên có:
- Số file nghi ngờ ban đầu
- Tiến độ hash (mỗi 1000 files)
- Số duplicate groups phát hiện được
- Tổng dung lượng duplicate

---

## 🎯 KẾT LUẬN

**Logic hiện tại:**
✅ Tối ưu về performance (loại bỏ N+1 queries)
✅ Batch processing hiệu quả
✅ Worker pool song song
❌ Thiếu bước đánh dấu duplicate ngay lập tức
❌ Không có thống kê real-time

**Khuyến nghị:**
1. Thêm cột `is_duplicate` vào database
2. Đánh dấu duplicate ngay sau khi update hash
3. Thêm thống kê và logging chi tiết hơn
4. Có thể tạo bảng `duplicate_groups` để query nhanh hơn

