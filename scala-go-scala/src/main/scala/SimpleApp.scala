import org.apache.spark.sql.SparkSession
import com.sun.jna._

import com.sun.jna.Structure
import java.util

object SimpleApp {
  def main(args: Array[String]): Unit = {
    // allegedly these options should allow passing objects to callbacks but meh
    // val options = new java.util.HashMap[String,Any]()
    // options.put(Library.OPTION_ALLOW_OBJECTS , java.lang.Boolean.TRUE);
    val tofromscala = Native
      .loadLibrary(
        "./tofromscala.so",
        classOf[SimpleJavaApp.Awesome] /*, options*/
      )
      .asInstanceOf[SimpleJavaApp.Awesome]

    printf("tofromscala.Add(12, 99) = %s\n", tofromscala.Add(12, 99))
    printf("tofromscala.Cosine(1.0) = %s\n", tofromscala.Cosine(1.0))

    // Call Sort
    // First, prepare data array
    val nums = Array[Long](53, 11, 5, 2, 88)
    val arr = new Memory(
      nums.length * Native.getNativeSize(java.lang.Long.TYPE)
    )
    arr.write(0, nums, 0, nums.length)
    // fill in the GoSlice class for type mapping
    val slice = new SimpleJavaApp.Awesome.GoSlice.ByValue
    slice.data = arr
    slice.len = nums.length
    slice.cap = nums.length
    tofromscala.Sort(slice)
    print("tofromscala.Sort(53,11,5,2,88) = [")
    val sorted = slice.data.getLongArray(0, nums.length)
    for (i <- 0 until sorted.length) {
      System.out.print(sorted(i) + " ")
    }
    System.out.println("]")

    // Call Log
    val str = new SimpleJavaApp.Awesome.GoString.ByValue
    str.p = "Hello Java!"
    str.n = str.p.length
    printf("msgid %d\n", tofromscala.Log(str))

    // The above is all based on "Calling Go from Other Languages", https://medium.com/learning-the-go-programming-language/calling-go-functions-from-other-languages-4c7d8bcc69bf .
    //
    // Now we add in callbacks back into the JVM.

    System.out.println("about to call you..")
    val callbackImpl = new SimpleJavaApp.Awesome.NotificationListenerImpl
    tofromscala.CallMe(callbackImpl)
    System.out.println("back in java")

    System.out.println("about to call non-CB WITH an object..")
    tofromscala.DoNothingWithArg(5.4) // surprise! dunno what this comes out as
    System.out.println("back in java")

    System.out.println("about to call you WITH a parameter...")
    val callbackParamImpl =
      new SimpleJavaApp.Awesome.NotificationParamListenerImpl
    tofromscala.CallMeWithSomething(callbackParamImpl, new NativeLong(9L))
    System.out.println("back in java, that worked")
  }
}
